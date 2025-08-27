package codegen

import (
	"embed"
	"io/fs"
	"path"
	"strings"
	"sync"
	"text/template"
)

var (
	//go:embed tmpl/*.tmpl
	Files embed.FS
)

var templateCache = make(map[string]*template.Template)
var templateCacheMux = &sync.RWMutex{}
var templateFuncs = template.FuncMap{
	"indent": func(s string, spaces int) string {
		lines := strings.Split(s, "\n")
		for i := range lines {
			if i > 0 && lines[i] != "" {
				lines[i] = strings.Repeat(" ", spaces) + lines[i]
			}
		}
		return strings.Join(lines, "\n")
	},
	"add": func(a, b int) int { return a + b },
	"index": func(slice []int, i int) int { return slice[i] },
}

// compile time check for templates
//var _ error = CompileTimeCheck(fs.FS(Files))

func GetTemplate(files ...string) *template.Template {
	name := strings.Join(files, "-")

	templateCacheMux.RLock()
	if templateCache[name] != nil {
		defer templateCacheMux.RUnlock()
		return templateCache[name]
	}
	templateCacheMux.RUnlock()

	tmpl := template.New(name).Funcs(template.FuncMap(templateFuncs))
	tmpl = template.Must(parseTemplateFiles(tmpl, readFileFS(Files), files...))
	templateCacheMux.Lock()
	defer templateCacheMux.Unlock()
	templateCache[name] = tmpl
	return templateCache[name]
}

func readFileFS(fsys fs.FS) func(string) (string, []byte, error) {
	return func(file string) (name string, b []byte, err error) {
		name = path.Base(file)
		b, err = fs.ReadFile(fsys, file)
		return
	}
}

func parseTemplateFiles(t *template.Template, readFile func(string) (string, []byte, error), filenames ...string) (*template.Template, error) {
	for _, filename := range filenames {
		name, b, err := readFile(filename)
		if err != nil {
			return nil, err
		}
		s := string(b)
		var tmpl *template.Template
		if t == nil {
			t = template.New(name)
		}
		if name == t.Name() {
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		_, err = tmpl.Parse(s)
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}
