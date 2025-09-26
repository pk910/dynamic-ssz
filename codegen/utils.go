package codegen

import (
	"strings"
)

func indentStr(s string, spaces int) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = strings.Repeat("\t", spaces) + lines[i]
		}
	}

	return strings.Join(lines, "\n")
}
