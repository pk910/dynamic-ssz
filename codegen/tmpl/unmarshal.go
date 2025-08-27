package tmpl

type UnmarshalMain struct {
	TypeName            string
	StaticSizeFunctions []*UnmarshalStaticSizeFunction
	UnmarshalFunctions  []*UnmarshalFunction
	RootFnName          string
	CreateLegacyFn      bool
	CreateDynamicFn     bool
	UsedDynSsz          bool
}

type UnmarshalFunction struct {
	Index      int
	Key        string
	Name       string
	TypeName   string
	InnerType  string
	Code       string
	IsPointer  bool
	UsedValue  bool
	IsInlined  bool
	InlineCode string
}

type UnmarshalStaticSizeFunction struct {
	Index    int
	Key      string
	Name     string
	TypeName string
	Code     string
}

type UnmarshalStaticSizeFastssz struct {
	TypeName string
}

type UnmarshalWrapper struct {
	TypeName    string
	UnmarshalFn string
}

type UnmarshalStaticSizeWrapper struct {
	TypeName string
	SizeFn   string
}

type UnmarshalPrimitive struct {
	TypeName string
}

type UnmarshalStruct struct {
	TypeName         string
	Fields           []UnmarshalField
	Size             int
	HasDynamicFields bool
	HasDynamicSizes  bool  // true if any field has dynamic size expressions
	StaticOffsets    []int // precomputed static offsets for each field
}

type UnmarshalStaticSizeStruct struct {
	TypeName string
	Fields   []UnmarshalStaticSizeField
	Size     int
}

type UnmarshalField struct {
	Index               int
	Name                string
	TypeName            string
	IsDynamic           bool
	Size                int
	UnmarshalFn         string
	InlineUnmarshalCode string
	SizeFn              *UnmarshalStaticSizeFunction
	NextDynamic         int
}

type UnmarshalStaticSizeField struct {
	Index    int
	Name     string
	TypeName string
	SizeFn   string
}

type UnmarshalVector struct {
	TypeName                string
	Length                  int
	ItemSize                int
	UnmarshalFn             string
	InlineItemUnmarshalCode string
	SizeFn                  *UnmarshalStaticSizeFunction
	ItemSizeFn              *UnmarshalStaticSizeFunction
	IsArray                 bool
	IsByteArray             bool
	IsString                bool
}

type UnmarshalStaticSizeVector struct {
	TypeName string
	Length   int
	ItemSize int
	SizeFn   string
	SizeExpr string
}

type UnmarshalDynamicVector struct {
	TypeName    string
	Length      int
	EmptySize   int
	UnmarshalFn string
	SizeExpr    string
	IsArray     bool
}

type UnmarshalList struct {
	TypeName                string
	ItemSize                int
	UnmarshalFn             string
	InlineItemUnmarshalCode string
	SizeFn                  *UnmarshalStaticSizeFunction
	SizeExpr                string
	IsByteArray             bool
	IsString                bool
}

type UnmarshalDynamicList struct {
	TypeName                string
	Length                  int
	EmptySize               int
	UnmarshalFn             string
	InlineItemUnmarshalCode string
	SizeExpr                string
	IsArray                 bool
}

type UnmarshalCompatibleUnion struct {
	TypeName   string
	VariantFns []UnmarshalCompatibleUnionVariant
}

type UnmarshalCompatibleUnionVariant struct {
	Index                int
	TypeName             string
	UnmarshalFn          string
	InlineUnmarshalCode  string
}
