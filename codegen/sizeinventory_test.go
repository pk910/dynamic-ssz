// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// Every site from which a size bound is emitted, per file. A size bounded on
// one path and not another is how a value gets past a limit on that path
// alone, and the shape it keeps taking is a helper reached from five of its
// six sites.
//
// A helper whose construct every generated path forms carries a rule: it must
// appear in each generator, so a generator that stops calling it fails here
// with that stated rather than as a number that moved. The rest record where
// the construct actually reaches, since it does not reach every path; what
// each bound refuses is held by the matrix in tests/sizeguard.
//
// gen_common.go holds the helpers the generators share and calls some of them
// itself, so it is read like any other file: a row that omits it asserts a
// helper is called nowhere while it is called there.
type emitterSites struct {
	// everyGenerator states that the construct this helper bounds is formed on
	// every generated path, so a generator missing it is a dropped bound.
	everyGenerator bool
	counts         map[string]int
}

var sizeEmitterFiles = []string{
	"gen_common.go", "gen_size.go", "gen_marshal.go", "gen_encoder.go",
	"gen_unmarshal.go", "gen_decoder.go", "gen_hashtreeroot.go",
}

// generators are the files that emit a method body, which is where a missing
// bound leaves a value unguarded; gen_common.go emits none of its own.
var sizeEmitterGenerators = sizeEmitterFiles[1:]

var sizeEmitterSites = map[string]emitterSites{
	// A big.Int payload is bounded on every path that reads or writes one.
	"bigIntLimit": {everyGenerator: true, counts: map[string]int{
		"gen_common.go": 0, "gen_size.go": 1, "gen_marshal.go": 1, "gen_encoder.go": 1,
		"gen_unmarshal.go": 1, "gen_decoder.go": 1, "gen_hashtreeroot.go": 1,
	}},
	// Three constructs: a container's static size and a declared vector size on
	// every path, and a list's element size only where a declaration is read.
	"platformGuard": {counts: map[string]int{
		"gen_common.go": 0, "gen_size.go": 4, "gen_marshal.go": 2, "gen_encoder.go": 2,
		"gen_unmarshal.go": 3, "gen_decoder.go": 3, "gen_hashtreeroot.go": 2,
	}},
	// An offset table is written only by the paths that write one.
	"appendListLenBound": {counts: map[string]int{
		"gen_common.go": 0, "gen_size.go": 0, "gen_marshal.go": 1, "gen_encoder.go": 1,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	}},
	// The shared prelude forms these, so every generator inherits them.
	"appendVectorLenBound": {counts: map[string]int{
		"gen_common.go": 2, "gen_size.go": 0, "gen_marshal.go": 0, "gen_encoder.go": 0,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	}},
	"appendSizeLimitCheck": {counts: map[string]int{
		"gen_common.go": 3, "gen_size.go": 0, "gen_marshal.go": 0, "gen_encoder.go": 0,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	}},
	// Only a sizer sums what a delegate reports.
	"appendDelegatedSize": {counts: map[string]int{
		"gen_common.go": 0, "gen_size.go": 5, "gen_marshal.go": 0, "gen_encoder.go": 0,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	}},
}

func TestSizeFormingEmitterSites(t *testing.T) {
	// Calls are counted from the syntax tree, so a name in a comment or in
	// commented-out code is not a site and a declaration is not a call.
	calls := make(map[string]map[string]int, len(sizeEmitterFiles))
	for _, file := range sizeEmitterFiles {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		counts := map[string]int{}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				counts[fn.Name]++
			case *ast.SelectorExpr:
				counts[fn.Sel.Name]++
			}

			return true
		})
		calls[file] = counts
	}

	helpers := make([]string, 0, len(sizeEmitterSites))
	for helper := range sizeEmitterSites {
		helpers = append(helpers, helper)
	}
	sort.Strings(helpers)

	for _, helper := range helpers {
		rule := sizeEmitterSites[helper]
		for _, file := range sizeEmitterFiles {
			want, recorded := rule.counts[file]
			if !recorded {
				t.Errorf("%s records no count for %s; every file the generators are built from needs one", helper, file)
				continue
			}
			if got := calls[file][helper]; got != want {
				t.Errorf("%s emits %s at %d sites, the inventory records %d.\n"+
					"  Route the new site, or record it and give it a row in codegen/tests' size-guard matrix.", file, helper, got, want)
			}
		}

		if !rule.everyGenerator {
			continue
		}
		for _, file := range sizeEmitterGenerators {
			if rule.counts[file] == 0 {
				t.Errorf("%s bounds a construct every generated path forms, but %s calls it nowhere", helper, file)
			}
		}
	}

	t.Log(inventoryTable(helpers))
}

func inventoryTable(helpers []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-22s", "emitter")
	for _, file := range sizeEmitterFiles {
		fmt.Fprintf(&b, " %8s", strings.TrimSuffix(strings.TrimPrefix(file, "gen_"), ".go"))
	}
	b.WriteString("\n")
	for _, helper := range helpers {
		fmt.Fprintf(&b, "%-22s", helper)
		for _, file := range sizeEmitterFiles {
			fmt.Fprintf(&b, " %8d", sizeEmitterSites[helper].counts[file])
		}
		b.WriteString("\n")
	}

	return b.String()
}
