// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every site from which a generator emits a size bound, counted per generator.
// A size that is formed on one path and not another is how a value gets past a
// limit on that path alone, and the shape it takes is a helper reached from
// five of its six sites. Counting them turns "the sixth was missed" into a
// failure here, and the matrix in codegen/tests names what each one refuses.
//
// A count that changes is not a defect by itself: add the site to the row it
// belongs to, or state why the construct does not reach that generator, then
// update the number.
var sizeEmitterSites = map[string]map[string]int{
	"bigIntLimit": {
		"gen_size.go": 1, "gen_marshal.go": 1, "gen_encoder.go": 1,
		"gen_unmarshal.go": 1, "gen_decoder.go": 1, "gen_hashtreeroot.go": 1,
	},
	"platformGuard": {
		"gen_size.go": 4, "gen_marshal.go": 2, "gen_encoder.go": 2,
		"gen_unmarshal.go": 3, "gen_decoder.go": 3, "gen_hashtreeroot.go": 2,
	},
	"appendListLenBound": {
		"gen_size.go": 0, "gen_marshal.go": 1, "gen_encoder.go": 1,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	},
	"appendVectorLenBound": {
		"gen_size.go": 0, "gen_marshal.go": 0, "gen_encoder.go": 0,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	},
	"appendSizeLimitCheck": {
		"gen_size.go": 0, "gen_marshal.go": 0, "gen_encoder.go": 0,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	},
	"appendDelegatedSize": {
		"gen_size.go": 5, "gen_marshal.go": 0, "gen_encoder.go": 0,
		"gen_unmarshal.go": 0, "gen_decoder.go": 0, "gen_hashtreeroot.go": 0,
	},
}

func TestSizeFormingEmitterSites(t *testing.T) {
	sources := make(map[string]string)
	for _, file := range []string{
		"gen_size.go", "gen_marshal.go", "gen_encoder.go",
		"gen_unmarshal.go", "gen_decoder.go", "gen_hashtreeroot.go",
	} {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		sources[file] = string(content)
	}

	helpers := make([]string, 0, len(sizeEmitterSites))
	for helper := range sizeEmitterSites {
		helpers = append(helpers, helper)
	}
	sort.Strings(helpers)

	for _, helper := range helpers {
		// A call, not a definition or a mention: the name followed by an open
		// paren, with any receiver in front of it.
		call := regexp.MustCompile(`(^|[^\w.])(\w+\.)?` + regexp.QuoteMeta(helper) + `\(`)
		for file, want := range sizeEmitterSites[helper] {
			got := 0
			for _, line := range strings.Split(sources[file], "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "func ") {
					// A comment mentions a helper and a declaration names one;
					// neither emits anything.
					continue
				}
				got += len(call.FindAllString(line, -1))
			}
			if got != want {
				t.Errorf("%s emits %s at %d sites, the inventory records %d.\n"+
					"  %s", file, helper, got, want,
					"Route the new site, or record it and give it a row in codegen/tests' size-guard matrix.")
			}
		}
	}

	t.Log(inventoryTable(helpers))
}

func inventoryTable(helpers []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %6s %6s %6s %6s %6s %6s\n",
		"emitter", "size", "marsh", "enc", "unmar", "dec", "htr")
	for _, helper := range helpers {
		row := sizeEmitterSites[helper]
		fmt.Fprintf(&b, "%-24s %6d %6d %6d %6d %6d %6d\n", helper,
			row["gen_size.go"], row["gen_marshal.go"], row["gen_encoder.go"],
			row["gen_unmarshal.go"], row["gen_decoder.go"], row["gen_hashtreeroot.go"])
	}
	return b.String()
}
