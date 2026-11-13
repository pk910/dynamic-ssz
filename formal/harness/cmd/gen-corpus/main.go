// gen-corpus writes the generic link's type corpus (formal/harness/corpus):
// Go types for every shape of the selected stages and the registry with the
// declaration each shape is meant to have.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pk910/dynamic-ssz/formal/harness/typegen"
)

func main() {
	out := flag.String("out", "corpus", "output directory (package corpus)")
	stages := flag.String("stages", "1,2,3", "stages to generate: 1 (every leaf as a root), 2 (every child class under every parent), 3 (inherited properties through a middle type), deleg (stage 1 plus method-less wrappers that delegate), rec (recursive shapes), random")
	seed := flag.Int64("seed", 1, "seed of the random stage")
	randomN := flag.Int("random", 50, "number of random shapes")
	randomDepth := flag.Int("depth", 4, "nesting depth of the random shapes")
	flag.Parse()

	var roots []*typegen.Root
	for _, st := range strings.Split(*stages, ",") {
		switch strings.TrimSpace(st) {
		case "1":
			roots = append(roots, typegen.Stage1()...)
		case "2":
			roots = append(roots, typegen.Stage2()...)
		case "3":
			roots = append(roots, typegen.Stage3()...)
		case "rec":
			roots = append(roots, typegen.Recursive()...)
		case "random":
			roots = append(roots, typegen.Random(*seed, *randomN, *randomDepth)...)
		case "deleg":
			// stage-1 roots with generated methods, each also held by a method-less wrapper
			s1 := typegen.Stage1()
			roots = append(roots, s1...)
			roots = append(roots, typegen.Delegating(s1)...)
		case "":
		default:
			fmt.Fprintf(os.Stderr, "unknown stage %q\n", st)
			os.Exit(2)
		}
	}
	em := typegen.NewEmitter()
	for _, r := range roots {
		if r.NoMethods {
			em.Delegating(r.Name, r.Inner)
			continue
		}
		em.Root(r)
	}
	src, err := em.Source("corpus")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.WriteFile(filepath.Join(*out, "types_gen.go.broken"), []byte(src), 0o600)
		os.Exit(1)
	}
	reg, err := typegen.Registry("corpus", roots)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*out, "types_gen.go"), []byte(src), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*out, "registry_gen.go"), []byte(reg), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*out, "types.txt"), []byte(strings.Join(em.Names, "\n")+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d types (%d stages: %s)\n", len(roots), len(strings.Split(*stages, ",")), *stages)
}
