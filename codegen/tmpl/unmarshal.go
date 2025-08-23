package tmpl

type UnmarshalMain struct {
	TypeName           string
	UnmarshalFunctions []*UnmarshalFunction
	RootFnName         string
	CreateLegacyFn     bool
	CreateDynamicFn    bool
	UsedDynSsz         bool
}

type UnmarshalFunction struct {
	Index     int
	Key       string
	Name      string
	TypeName  string
	InnerType string
	Code      string
	IsPointer bool
	UsedValue bool
}

type UnmarshalWrapper struct {
	TypeName    string
	UnmarshalFn string
}

type UnmarshalPrimitive struct {
	TypeName string
}

type UnmarshalStruct struct {
	TypeName         string
	Fields           []UnmarshalField
	Size             int
	HasDynamicFields bool
}

type UnmarshalField struct {
	Index       int
	Name        string
	TypeName    string
	IsDynamic   bool
	Size        int
	UnmarshalFn string
	NextDynamic int
}

type UnmarshalVector struct {
	TypeName    string
	Length      int
	ItemSize    int
	UnmarshalFn string
	SizeExpr    string
	IsArray     bool
	IsByteArray bool
	IsString    bool
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
	TypeName    string
	ItemSize    int
	UnmarshalFn string
	SizeExpr    string
	IsByteArray bool
	IsString    bool
}

type UnmarshalDynamicList struct {
	TypeName    string
	Length      int
	EmptySize   int
	UnmarshalFn string
	SizeExpr    string
	IsArray     bool
}
