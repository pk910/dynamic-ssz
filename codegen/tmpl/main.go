package tmpl

type Main struct {
	PackageName string
	Imports     []TypeImport
	Code        string
}

type TypeImport struct {
	Alias string
	Path  string
}
