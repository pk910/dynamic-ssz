package tmpl

type MarshalMain struct {
	TypeName         string
	MarshalFunctions []*MarshalFunction
	RootFnName       string
	CreateLegacyFn   bool
	CreateDynamicFn  bool
	UsedDynSsz       bool
}

type MarshalFunction struct {
	Index      int
	Key        string
	Name       string
	TypeName   string
	Code       string
	IsInlined  bool
	InlineCode string
}

type MarshalWrapper struct {
	TypeName  string
	MarshalFn string
}

type MarshalStruct struct {
	TypeName         string
	Fields           []MarshalField
	HasDynamicFields bool
}

type MarshalField struct {
	Index             int
	Name              string
	TypeName          string
	IsDynamic         bool
	Size              int
	MarshalFn         string
	InlineMarshalCode string
}

type MarshalVector struct {
	TypeName              string
	Length                int
	ItemSize              int
	MarshalFn             string
	InlineItemMarshalCode string
	SizeExpr              string
	IsArray               bool
	IsByteArray           bool
	IsString              bool
}

type MarshalDynamicVector struct {
	TypeName              string
	Length                int
	EmptySize             int
	MarshalFn             string
	InlineItemMarshalCode string
	SizeExpr              string
	IsArray               bool
}

type MarshalList struct {
	TypeName              string
	ItemSize              int
	MarshalFn             string
	InlineItemMarshalCode string
	SizeExpr              string
	IsByteArray           bool
	IsString              bool
}

type MarshalDynamicList struct {
	TypeName              string
	Length                int
	EmptySize             int
	MarshalFn             string
	InlineItemMarshalCode string
	SizeExpr              string
	IsArray               bool
}

type MarshalCompatibleUnion struct {
	TypeName   string
	VariantFns []MarshalCompatibleUnionVariant
}

type MarshalCompatibleUnionVariant struct {
	Index             int
	TypeName          string
	MarshalFn         string
	InlineMarshalCode string
}
