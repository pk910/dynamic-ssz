package harness

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// subject is one certified type of the link.
type subject struct {
	name  string
	newFn func() any
	spec  bool // has an EthLib declaration (formal/spec)
}

// standaloneTypes are certified types that no fixture registry lists (they
// appear only nested in fixture types).
var standaloneTypes = map[string]func() any{
	"CUSquareProgList": func() any { return new(CUSquareProgList) },
	"CUCircleProgList": func() any { return new(CUCircleProgList) },
	"CUUint16List4":    func() any { return new(CUUint16List4) },
}

// certifiedSubjects returns every type listed in certified-types.txt (the
// list certify.sh certifies), each with its constructor and whether the
// official specification declares it.
func certifiedSubjects(t testing.TB) []subject {
	ctors := map[string]func() any{}
	for name, fn := range standaloneTypes {
		ctors[name] = fn
	}
	for _, reg := range Registries {
		for _, entry := range reg {
			name := reflect.TypeOf(entry.New()).Elem().Name()
			if !strings.Contains(name, "[") {
				ctors[name] = entry.New
			}
		}
	}
	specDecl, err := os.ReadFile(filepath.Join("..", "spec", "SszSpec", "Certified.lean"))
	if err != nil {
		t.Fatalf("read spec declarations: %v", err)
	}
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^def D_([A-Za-z0-9_]+) `).FindAllStringSubmatch(string(specDecl), -1) {
		declared[m[1]] = true
	}
	f, err := os.Open("certified-types.txt")
	if err != nil {
		t.Fatalf("open certified-types.txt: %v", err)
	}
	defer f.Close()
	var subjects []subject
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		fn, ok := ctors[name]
		if !ok {
			t.Fatalf("certified type %s has no constructor in the registries", name)
		}
		subjects = append(subjects, subject{name: name, newFn: fn, spec: declared[name]})
	}
	return subjects
}

// genValue draws a random value of the subject's type from the descriptor
// the reflection engine builds for it.
func (s subject) genValue(t testing.TB, rt *rapid.T, e *engineSet) any {
	v, err := GenValue(rt, e.refl, s.newFn, DefaultGen)
	if err != nil {
		t.Fatalf("generate %s: %v", s.name, err)
	}
	return v
}
