package tmpl

type HashTreeRootMain struct {
	TypeName              string
	HasherAlias           string
	HashTreeRootFunctions []*HashTreeRootFunction
	RootFnName            string
	CreateLegacyFn        bool
	CreateDynamicFn       bool
	UsedDynSsz            bool
}

type HashTreeRootFunction struct {
	Index      int
	Key        string
	Name       string
	TypeName   string
	Code       string
	IsInlined  bool
	InlineCode string
	IsPointer  bool
	InnerType  string
}

type HashTreeRootWrapper struct {
	TypeName       string
	HashTreeRootFn string
}

type HashTreeRootStruct struct {
	TypeName         string
	Fields           []HashTreeRootField
	HasDynamicFields bool
}

type HashTreeRootField struct {
	Index          int
	Name           string
	TypeName       string
	IsDynamic      bool
	HashTreeRootFn string
	InlineHashCode string
	SszIndex       uint16
}

type HashTreeRootVector struct {
	TypeName           string
	Length             int
	ItemSize           int
	HashTreeRootFn     string
	InlineItemHashCode string
	SizeExpr           string
	IsArray            bool
	IsByteArray        bool
	IsString           bool
}

type HashTreeRootList struct {
	TypeName           string
	ItemSize           int
	MaxLength          int
	HashTreeRootFn     string
	InlineItemHashCode string
	SizeExpr           string
	MaxExpr            string
	HasLimit           bool
	IsProgressive      bool
	IsByteArray        bool
	IsString           bool
}

type HashTreeRootBitlist struct {
	TypeName      string
	MaxLength     int
	MaxExpr       string
	HasLimit      bool
	IsProgressive bool
}

type HashTreeRootCompatibleUnion struct {
	TypeName   string
	VariantFns []HashTreeRootCompatibleUnionVariant
}

type HashTreeRootCompatibleUnionVariant struct {
	Index          int
	TypeName       string
	HashTreeRootFn string
	InlineHashCode string
}

type HashTreeRootProgressiveContainer struct {
	TypeName     string
	ActiveFields string // Hex representation of active fields bitlist
	Fields       []HashTreeRootField
}
