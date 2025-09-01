package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

type CodeGeneratorOption func(*CodeGeneratorOptions)

type CodeGeneratorOptions struct {
	NoMarshalSSZ              bool
	NoUnmarshalSSZ            bool
	NoSizeSSZ                 bool
	NoHashTreeRoot            bool
	CreateLegacyFn            bool
	WithoutDynamicExpressions bool
	SizeHints                 []dynssz.SszSizeHint
	MaxSizeHints              []dynssz.SszMaxSizeHint
	TypeHints                 []dynssz.SszTypeHint
	Types                     []CodeGeneratorTypeOption
}

type CodeGeneratorTypeOption struct {
	Type reflect.Type
	Opts []CodeGeneratorOption
}

type CodeGeneratorTypeOptions struct {
	Type       reflect.Type
	Options    CodeGeneratorOptions
	Descriptor *dynssz.TypeDescriptor
}

type CodeGeneratorFileOptions struct {
	Package string
	Types   []*CodeGeneratorTypeOptions
}

func WithNoMarshalSSZ() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoMarshalSSZ = true
	}
}

func WithNoUnmarshalSSZ() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoUnmarshalSSZ = true
	}
}

func WithNoSizeSSZ() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoSizeSSZ = true
	}
}

func WithNoHashTreeRoot() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoHashTreeRoot = true
	}
}

func WithSizeHints(hints []dynssz.SszSizeHint) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.SizeHints = hints
	}
}

func WithMaxSizeHints(hints []dynssz.SszMaxSizeHint) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.MaxSizeHints = hints
	}
}

// WithTypeHints creates code with type hints for the dynamic expressions
// this is useful to generate code that is compatible with the dynamic expressions
func WithTypeHints(hints []dynssz.SszTypeHint) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.TypeHints = hints
	}
}

// WithCreateLegacyFn creates code with legacy methods that use the global dynssz instance
// this is useful to generate code that is compatible with the legacy fastssz interfaces
func WithCreateLegacyFn() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.CreateLegacyFn = true
	}
}

// WithoutDynamicExpressions creates code that uses static sizes only and ignores dynamic expressions
// this is useful to generate code with maximum performance characteristics for the default preset, while maintaining the expression flexibility for other presets via the slower reflection-based methods
// this option is not compatible with WithCreateDynamicFn
func WithoutDynamicExpressions() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.WithoutDynamicExpressions = true
	}
}

func WithType(t reflect.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.Types = append(opts.Types, CodeGeneratorTypeOption{
			Type: t,
			Opts: typeOpts,
		})
	}
}

// fileGenerationRequest represents a request to generate a file with SSZ methods for specific types
type fileGenerationRequest struct {
	FileName string
	Options  *CodeGeneratorFileOptions
}

// CodeGenerator manages batch generation of SSZ methods for multiple types
type CodeGenerator struct {
	files  []*fileGenerationRequest
	dynSsz *dynssz.DynSsz
}

// NewCodeGenerator creates a new code generator instance
func NewCodeGenerator(dynSsz *dynssz.DynSsz) *CodeGenerator {
	return &CodeGenerator{
		files:  make([]*fileGenerationRequest, 0),
		dynSsz: dynSsz,
	}
}

func (cg *CodeGenerator) BuildFile(fileName string, opts ...CodeGeneratorOption) {
	baseCodeOpts := CodeGeneratorOptions{}
	for _, opt := range opts {
		opt(&baseCodeOpts)
	}

	fileOpts := CodeGeneratorFileOptions{}

	types := baseCodeOpts.Types
	baseCodeOpts.Types = nil

	for _, t := range types {
		codeOpts := baseCodeOpts
		for _, opt := range t.Opts {
			opt(&codeOpts)
		}

		fileOpts.Types = append(fileOpts.Types, &CodeGeneratorTypeOptions{
			Type:    t.Type,
			Options: codeOpts,
		})
	}

	cg.files = append(cg.files, &fileGenerationRequest{
		FileName: fileName,
		Options:  &fileOpts,
	})
}

// GenerateToMap generates code for all requested types and returns it as a map of file name to code
func (cg *CodeGenerator) GenerateToMap() (map[string]string, error) {
	if len(cg.files) == 0 {
		return nil, fmt.Errorf("no types requested for generation")
	}

	// analyze all types to build complete dependency graph
	for _, file := range cg.files {
		pkgPath := ""
		for _, t := range file.Options.Types {
			typePkgPath := t.Type.PkgPath()
			if typePkgPath == "" && t.Type.Kind() == reflect.Ptr {
				typePkgPath = t.Type.Elem().PkgPath()
			}
			if typePkgPath == "" {
				return nil, fmt.Errorf("type %s has no package path", t.Type.Name())
			}
			if pkgPath == "" {
				pkgPath = typePkgPath
			} else if pkgPath != typePkgPath {
				return nil, fmt.Errorf("type %s has different package path than %s. cannot combine types from different packages in a single file", t.Type.Name(), file.Options.Types[0].Type.Name())
			}

			desc, err := cg.dynSsz.GetTypeCache().GetTypeDescriptor(t.Type, t.Options.SizeHints, t.Options.MaxSizeHints, t.Options.TypeHints)
			if err != nil {
				return nil, fmt.Errorf("failed to analyze type %s: %w", t.Type.Name(), err)
			}

			// set availability of dynamic methods (we will generate them in a bit and we want cross references)
			if !t.Options.NoMarshalSSZ && !t.Options.WithoutDynamicExpressions {
				desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicMarshaler
			}
			if !t.Options.NoUnmarshalSSZ && !t.Options.WithoutDynamicExpressions {
				desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicUnmarshaler
			}
			if !t.Options.NoSizeSSZ && !t.Options.WithoutDynamicExpressions {
				desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicSizer
			}
			if !t.Options.NoHashTreeRoot && !t.Options.WithoutDynamicExpressions {
				desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicHashRoot
			}

			if !t.Options.NoMarshalSSZ && !t.Options.NoUnmarshalSSZ && !t.Options.NoSizeSSZ && (t.Options.CreateLegacyFn || t.Options.WithoutDynamicExpressions) {
				desc.SszCompatFlags |= dynssz.SszCompatFlagFastSSZMarshaler
			}
			if !t.Options.NoHashTreeRoot && (t.Options.CreateLegacyFn || t.Options.WithoutDynamicExpressions) {
				desc.SszCompatFlags |= dynssz.SszCompatFlagFastSSZHasher
			}

			t.Descriptor = desc
		}

		file.Options.Package = pkgPath
	}

	// generate code for each file
	results := make(map[string]string)
	for _, file := range cg.files {
		code, err := cg.generateFile(file.FileName, file.Options.Package, file.Options)
		if err != nil {
			return nil, fmt.Errorf("failed to generate code for %s: %w", file.FileName, err)
		}

		results[file.FileName] = code
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

func (cg *CodeGenerator) generateFile(fileName string, packagePath string, opts *CodeGeneratorFileOptions) (string, error) {
	if len(opts.Types) == 0 {
		return "", fmt.Errorf("no types requested for generation")
	}

	typePrinter := NewTypePrinter(packagePath)
	typePrinter.AddAlias("github.com/pk910/dynamic-ssz", "dynssz")
	usedDynSsz := false
	codeBuilder := strings.Builder{}
	hashParts := [][]byte{}

	for _, t := range opts.Types {
		if t.Descriptor == nil {
			return "", fmt.Errorf("type %s has no descriptor", t.Type.Name())
		}

		hash, err := t.Descriptor.GetTypeHash()
		if err != nil {
			return "", fmt.Errorf("failed to get type hash for %s: %w", t.Type.Name(), err)
		}
		hashParts = append(hashParts, hash[:])

		withDynSsz, err := cg.generateCode(t.Descriptor, typePrinter, &codeBuilder, &t.Options)
		if err != nil {
			return "", fmt.Errorf("failed to generate code for %s: %w", t.Type.Name(), err)
		}
		usedDynSsz = usedDynSsz || withDynSsz
	}

	typesHash := sha256.Sum256(bytes.Join(hashParts, []byte{}))

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
	pkgName := packagePath
	if slashIdx := strings.Index(pkgName, "/"); slashIdx != -1 {
		pkgName = pkgName[slashIdx+1:]
	}

	mainCode := tmpl.Main{
		PackageName: pkgName,
		TypesHash:   hex.EncodeToString(typesHash[:]),
		Version:     Version,
		Imports:     imports,
		Code:        codeBuilder.String(),
	}

	mainCodeTpl := GetTemplate("tmpl/main.tmpl")
	mainCodeBuilder := strings.Builder{}
	if err := mainCodeTpl.ExecuteTemplate(&mainCodeBuilder, "main", mainCode); err != nil {
		return "", fmt.Errorf("failed to generate code for %s: %w", fileName, err)
	}

	return mainCodeBuilder.String(), nil
}

// generateCode generates the code for a single type
func (cg *CodeGenerator) generateCode(desc *dynssz.TypeDescriptor, typePrinter *TypePrinter, codeBuilder *strings.Builder, options *CodeGeneratorOptions) (bool, error) {
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

	if !options.NoHashTreeRoot {
		usedDynSsz, err = generateHashTreeRoot(cg.dynSsz, desc, codeBuilder, typePrinter, options)
		if err != nil {
			return usedDynSsz, fmt.Errorf("failed to generate hash tree root for %s: %w", desc.Type.Name(), err)
		}
		usedDynSszResult = usedDynSszResult || usedDynSsz
	}

	return usedDynSszResult, nil
}
