package tmpl

type Main struct {
	PackageName string
	TypesHash   string
	Version     string
	Imports     []TypeImport
	Code        string
}

type TypeImport struct {
	Alias string
	Path  string
}
