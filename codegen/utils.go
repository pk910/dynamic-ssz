// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"strconv"
	"strings"
)

// appendCode appends a formatted code string to a strings.Builder with proper indentation.
//
// This function is used to append formatted code to a strings.Builder with the specified
// indentation level. It handles string formatting with optional arguments and ensures
// that each line is properly indented.
//
// Parameters:
//   - codeBuf: The strings.Builder to append the code to
//   - indent: The number of tab characters to prepend to each non-empty line
//   - code: The format string to append
//   - args: Optional arguments to format the code string
//
// Returns:
//   - None
//
// Example:
//
//	codeBuf := strings.Builder{}
//	appendCode(&codeBuf, 1, "func example() {\nreturn nil\n}")
//	// Result: "\tfunc example() {\n\treturn nil\n\t}"
func appendCode(codeBuf *strings.Builder, indent int, code string, args ...any) {
	if len(args) > 0 {
		code = fmt.Sprintf(code, args...)
	}
	codeBuf.WriteString(indentStr(code, indent))
}

// indentStr indents each non-empty line in a string by the specified number of tab characters.
//
// This utility function is used during code generation to properly format generated Go code
// with consistent indentation. Empty lines are left unchanged to preserve code structure.
//
// Parameters:
//   - s: The input string to indent (may contain multiple lines)
//   - spaces: The number of tab characters to prepend to each non-empty line
//
// Returns:
//   - string: The input string with each non-empty line indented by the specified amount
//
// Example:
//
//	code := "func example() {\nreturn nil\n}"
//	indented := indentStr(code, 1)
//	// Result: "\tfunc example() {\n\treturn nil\n\t}"
func indentStr(s string, spaces int) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if lines[i] != "" {
			lines[i] = strings.Repeat("\t", spaces) + lines[i]
		}
	}

	return strings.Join(lines, "\n")
}

// escapeBackticks properly escapes backtick characters for use in generated Go string literals.
//
// Go raw string literals (enclosed in backticks) cannot contain backtick characters.
// When generating code that includes strings with backticks (such as struct tags),
// this function converts them to quoted string literals with proper escaping.
//
// The function uses strconv.Quote to handle the escaping and then removes the outer
// quotes, allowing the result to be embedded in larger string constructions.
//
// Parameters:
//   - s: The input string that may contain backtick characters
//
// Returns:
//   - string: The input string with backticks properly escaped for Go code generation
//     If no backticks are present, returns the original string unchanged
//
// Example:
//
//	tag := "`json:\"field\" ssz:\"vector,32\"`"
//	escaped := escapeBackticks(tag)
//	// Result: "json:\"field\" ssz:\"vector,32\""
func escapeBackticks(s string) string {
	// Backticks cannot appear in raw string literals; encode as a quoted + strconv backtick
	if strings.Contains(s, "`") {
		return strconv.Quote(s)[1 : len(strconv.Quote(s))-1] // \"...\" sans outer quotes
	}
	return s
}

// escapeViewFnName escapes a view function name for use in generated Go code.
//
// This function replaces all dots, dashes, and slashes in the view name
// with underscores to avoid conflicts with package names and function names.
//
// Parameters:
//   - viewName: The input view name to escape
//
// Returns:
//   - string: The escaped view function name with dots replaced by underscores
//
// Example:
//
//	viewName := "pkg.View2"
//	escaped := escapeViewFnName(viewName)
//	// Result: "pkg_View2"
func escapeViewFnName(viewName string) string {
	viewName = strings.ReplaceAll(viewName, ".", "_")
	viewName = strings.ReplaceAll(viewName, "-", "_")
	viewName = strings.ReplaceAll(viewName, "/", "_")
	return viewName
}
