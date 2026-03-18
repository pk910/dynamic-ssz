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
	// Write directly into the target builder so common codegen paths avoid
	// creating an extra indented string first.
	writeIndented(codeBuf, code, indent)
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
	if spaces <= 0 || s == "" {
		return s
	}

	var b strings.Builder
	// Reserve enough room for the original string plus one indent prefix per line.
	b.Grow(len(s) + (spaces * (strings.Count(s, "\n") + 1)))
	writeIndented(&b, s, spaces)
	return b.String()
}

func writeIndented(codeBuf *strings.Builder, s string, spaces int) {
	if spaces <= 0 || s == "" {
		codeBuf.WriteString(s)
		return
	}

	prefix := strings.Repeat("\t", spaces)
	lineStart := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' {
			continue
		}
		if i > lineStart {
			codeBuf.WriteString(prefix)
			codeBuf.WriteString(s[lineStart:i])
		}
		codeBuf.WriteByte('\n')
		lineStart = i + 1
	}

	if lineStart < len(s) {
		codeBuf.WriteString(prefix)
		codeBuf.WriteString(s[lineStart:])
	}
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
		quoted := strconv.Quote(s)
		return quoted[1 : len(quoted)-1] // \"...\" sans outer quotes
	}
	return s
}
