package codegen

import (
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

type CodeGenOption func(*CodeGenOptions)

type CodeGenOptions struct {
	DynSSZ          *dynssz.DynSsz
	NoMarshalSSZ    bool
	NoUnmarshalSSZ  bool
	NoSizeSSZ       bool
	NoHashTreeRoot  bool
	CreateLegacyFn  bool
	CreateDynamicFn bool
	SizeHints       []dynssz.SszSizeHint
	MaxSizeHints    []dynssz.SszMaxSizeHint
	TypeHints       []dynssz.SszTypeHint
}

func WithDynSSZ(ds *dynssz.DynSsz) CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.DynSSZ = ds
	}
}

func WithNoMarshalSSZ() CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.NoMarshalSSZ = true
	}
}

func WithNoUnmarshalSSZ() CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.NoUnmarshalSSZ = true
	}
}

func WithNoSizeSSZ() CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.NoSizeSSZ = true
	}
}

func WithNoHashTreeRoot() CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.NoHashTreeRoot = true
	}
}

func WithSizeHints(hints []dynssz.SszSizeHint) CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.SizeHints = hints
	}
}

func WithMaxSizeHints(hints []dynssz.SszMaxSizeHint) CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.MaxSizeHints = hints
	}
}

func WithTypeHints(hints []dynssz.SszTypeHint) CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.TypeHints = hints
	}
}

func WithCreateLegacyFn() CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.CreateLegacyFn = true
	}
}

func WithCreateDynamicFn() CodeGenOption {
	return func(opts *CodeGenOptions) {
		opts.CreateDynamicFn = true
	}
}

func GenerateSSZCode(source any, opts ...CodeGenOption) (string, error) {
	options := &CodeGenOptions{}
	for _, opt := range opts {
		opt(options)
	}

	sourceType := reflect.TypeOf(source)

	ds := options.DynSSZ
	if ds == nil {
		ds = dynssz.NewDynSsz(nil)
	}

	sourceTypeDesc, err := ds.GetTypeCache().GetTypeDescriptor(sourceType, options.SizeHints, options.MaxSizeHints, options.TypeHints)
	if err != nil {
		return "", err
	}

	rootType := sourceTypeDesc.Type
	if sourceTypeDesc.IsPtr {
		rootType = rootType.Elem()
	}

	typePrinter := NewTypePrinter(rootType.PkgPath())
	typePrinter.AddAlias("github.com/pk910/dynamic-ssz", "dynssz")
	usedDynSsz := options.CreateDynamicFn

	// generate MarshalSSZ code
	marshalCode := strings.Builder{}
	if !options.NoMarshalSSZ {
		usedDynSszFn, err := generateMarshal(ds, sourceTypeDesc, &marshalCode, typePrinter, options)
		if err != nil {
			return "", err
		}
		usedDynSsz = usedDynSsz || usedDynSszFn
	}

	// generate SizeSSZ code
	sizeCode := strings.Builder{}
	if !options.NoSizeSSZ {
		usedDynSszFn, err := generateSize(ds, sourceTypeDesc, &sizeCode, typePrinter, options)
		if err != nil {
			return "", err
		}
		usedDynSsz = usedDynSsz || usedDynSszFn
	}

	// generate UnmarshalSSZ code
	unmarshalCode := strings.Builder{}
	if !options.NoUnmarshalSSZ {
		usedDynSszFn, err := generateUnmarshal(ds, sourceTypeDesc, &unmarshalCode, typePrinter, options)
		if err != nil {
			return "", err
		}
		usedDynSsz = usedDynSsz || usedDynSszFn
	}

	// add base imports
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
	pkgName := rootType.PkgPath()
	if slashIdx := strings.Index(pkgName, "/"); slashIdx != -1 {
		pkgName = pkgName[slashIdx+1:]
	}

	mainCode := tmpl.Main{
		PackageName: pkgName,
		Imports:     imports,
		Code:        marshalCode.String() + "\n" + sizeCode.String() + "\n" + unmarshalCode.String(),
	}

	mainCodeTpl := GetTemplate("tmpl/main.tmpl")
	mainCodeBuilder := strings.Builder{}
	if err := mainCodeTpl.ExecuteTemplate(&mainCodeBuilder, "main", mainCode); err != nil {
		return "", err
	}

	return mainCodeBuilder.String(), nil
}
