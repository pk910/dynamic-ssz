// Package codegen provides a flexible API for generating SSZ marshal/unmarshal/size methods
// for Go types. This API allows users to create simple main() functions that specify
// which types to generate and where to save the output.
package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

// GenerationRequest represents a request to generate SSZ methods for a specific type
type GenerationRequest struct {
	FileName string
	Types    []reflect.Type // The types to generate methods for
	Package  string         // Package name for the generated code
}

// CodeGenerator manages batch generation of SSZ methods for multiple types
type CodeGenerator struct {
	requests    []*GenerationRequest
	dynSsz      *dynssz.DynSsz
	options     *CodeGenOptions
	typePrinter *TypePrinter
}

// NewCodeGenerator creates a new code generator instance
func NewCodeGenerator(dynSsz *dynssz.DynSsz, opts ...CodeGenOption) *CodeGenerator {
	options := &CodeGenOptions{
		CreateDynamicFn: true,
	}
	for _, opt := range opts {
		opt(options)
	}

	return &CodeGenerator{
		requests:    make([]*GenerationRequest, 0),
		dynSsz:      dynSsz,
		options:     options,
		typePrinter: NewTypePrinter(""),
	}
}

func (cg *CodeGenerator) BuildFile(fileName string, types ...reflect.Type) error {
	pkgName := ""

	for _, t := range types {
		tpkgPath := t.PkgPath()
		if tpkgPath == "" && t.Kind() == reflect.Ptr {
			tpkgPath = t.Elem().PkgPath()
		}
		if tpkgPath == "" {
			return fmt.Errorf("type %s has no package path", t.Name())
		}
		if pkgName == "" {
			pkgName = tpkgPath
		} else if pkgName != tpkgPath {
			return fmt.Errorf("type %s has different package path than %s. cannot combine types from different packages in a single file", t.Name(), types[0].Name())
		}
	}

	cg.requests = append(cg.requests, &GenerationRequest{
		FileName: fileName,
		Types:    types,
		Package:  pkgName,
	})

	return nil
}

// GenerateToMap generates code for all requested types and returns it as a map of file name to code
func (cg *CodeGenerator) GenerateToMap() (map[string]string, error) {
	if len(cg.requests) == 0 {
		return nil, fmt.Errorf("no types requested for generation")
	}

	// First, analyze all types to build complete dependency graph
	typeMap := make(map[reflect.Type]*dynssz.TypeDescriptor)
	for _, req := range cg.requests {
		for _, t := range req.Types {
			desc, err := cg.dynSsz.GetTypeCache().GetTypeDescriptor(t, nil, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to analyze type %s: %w", t.Name(), err)
			}

			// set availability of dynamic methods (we will generate them in a bit and we want cross references)
			desc.HasDynamicMarshaler = true
			desc.HasDynamicUnmarshaler = true
			desc.HasDynamicSizer = true

			typeMap[t] = desc
		}
	}

	results := make(map[string]string)

	for _, req := range cg.requests {
		typePrinter := NewTypePrinter(req.Package)
		typePrinter.AddAlias("github.com/pk910/dynamic-ssz", "dynssz")
		usedDynSsz := cg.options.CreateDynamicFn
		codeBuilder := strings.Builder{}

		for _, t := range req.Types {

			desc := typeMap[t]
			withDynSsz, err := cg.generateCode(desc, typePrinter, &codeBuilder, cg.options)
			if err != nil {
				return nil, fmt.Errorf("failed to generate code for %s: %w", t.Name(), err)
			}
			usedDynSsz = usedDynSsz || withDynSsz
		}

		if usedDynSsz {
			typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
		}
		typePrinter.AddImport("github.com/pk910/dynamic-ssz/sszutils", "sszutils")

		// collect & sort imports
		importsMap := typePrinter.Imports()
		imports := make([]tmpl.TypeImport, 0, len(importsMap))
		for path, alias := range importsMap {
			if presetAlias := typePrinter.Aliases()[path]; presetAlias != "" {
				alias = presetAlias
			} else if defaultAlias := typePrinter.defaultAlias(path); alias == defaultAlias {
				alias = ""
			}
			imports = append(imports, tmpl.TypeImport{
				Alias: alias,
				Path:  path,
			})
		}

		sort.Slice(imports, func(i, j int) bool {
			return imports[i].Path < imports[j].Path
		})

		// generate main code
		pkgName := req.Package
		if slashIdx := strings.Index(pkgName, "/"); slashIdx != -1 {
			pkgName = pkgName[slashIdx+1:]
		}

		mainCode := tmpl.Main{
			PackageName: pkgName,
			Imports:     imports,
			Code:        codeBuilder.String(),
		}

		mainCodeTpl := GetTemplate("tmpl/main.tmpl")
		mainCodeBuilder := strings.Builder{}
		if err := mainCodeTpl.ExecuteTemplate(&mainCodeBuilder, "main", mainCode); err != nil {
			return nil, fmt.Errorf("failed to generate code for %s: %w", req.FileName, err)
		}

		results[req.FileName] = mainCodeBuilder.String()
	}

	return results, nil
}

func (cg *CodeGenerator) Generate() error {
	results, err := cg.GenerateToMap()
	if err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	for fileName, code := range results {
		dir := filepath.Dir(fileName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(fileName, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write code to file %s: %w", fileName, err)
		}
	}

	return nil
}

// generateCode generates the code for a single type
func (cg *CodeGenerator) generateCode(desc *dynssz.TypeDescriptor, typePrinter *TypePrinter, codeBuilder *strings.Builder, options *CodeGenOptions) (bool, error) {
	// Generate the actual methods
	var err error
	var usedDynSsz bool
	var usedDynSszResult bool

	usedDynSsz, err = generateMarshal(cg.dynSsz, desc, codeBuilder, typePrinter, options)
	if err != nil {
		return usedDynSsz, fmt.Errorf("failed to generate marshal for %s: %w", desc.Type.Name(), err)
	}
	usedDynSszResult = usedDynSsz

	usedDynSsz, err = generateSize(cg.dynSsz, desc, codeBuilder, typePrinter, options)
	if err != nil {
		return usedDynSsz, fmt.Errorf("failed to generate size for %s: %w", desc.Type.Name(), err)
	}
	usedDynSszResult = usedDynSszResult || usedDynSsz

	usedDynSsz, err = generateUnmarshal(cg.dynSsz, desc, codeBuilder, typePrinter, options)
	if err != nil {
		return usedDynSsz, fmt.Errorf("failed to generate unmarshal for %s: %w", desc.Type.Name(), err)
	}
	usedDynSszResult = usedDynSszResult || usedDynSsz

	return usedDynSszResult, nil
}
