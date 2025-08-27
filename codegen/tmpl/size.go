package tmpl

type SizeMain struct {
	TypeName        string
	SizeFunctions   []*SizeFunction
	RootFnName      string
	CreateLegacyFn  bool
	CreateDynamicFn bool
	UsedDynSsz      bool
}

type SizeFunction struct {
	Index     int
	Key       string
	Name      string
	TypeName  string
	InnerType string
	Code      string
	IsPointer bool
}

type SizeWrapper struct {
	TypeName string
	SizeFn   string
}

type SizeStruct struct {
	TypeName         string
	Fields           []SizeField
	Size             int
	HasDynamicFields bool
}

type SizeField struct {
	Index     int
	Name      string
	TypeName  string
	IsDynamic bool
	SizeFn    string
}

type SizeVector struct {
	TypeName    string
	Length      int
	ItemSize    int
	SizeFn      string
	SizeExpr    string
	IsArray     bool
	IsByteArray bool
	IsString    bool
}

type SizeDynamicVector struct {
	TypeName  string
	Length    int
	EmptySize int
	SizeFn    string
	SizeExpr  string
	IsArray   bool
}

type SizeList struct {
	TypeName    string
	ItemSize    int
	SizeFn      string
	SizeExpr    string
	IsByteArray bool
	IsString    bool
}

type SizeDynamicList struct {
	TypeName  string
	Length    int
	EmptySize int
	SizeFn    string
	SizeExpr  string
	IsArray   bool
}

type SizeCompatibleUnion struct {
	TypeName   string
	VariantFns []SizeCompatibleUnionVariant
}

type SizeCompatibleUnionVariant struct {
	Index    int
	TypeName string
	SizeFn   string
}
