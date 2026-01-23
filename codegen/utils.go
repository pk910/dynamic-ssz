// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
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

// getTypeName returns the type name and package path for a given go/types type or reflection type.
//
// Parameters:
//   - goType: The go/types type to get the type name and package path for
//   - reflectType: The reflection type to get the type name and package path for
//
// Returns:
//   - string: The type name
//   - string: The package path
//
// Example:
//
//	typeName, pkgPath := getTypeName(goType, reflectType)
func getTypeName(goType types.Type, reflectType reflect.Type) (string, string) {
	var typeName, typePkgPath string
	if reflectType != nil {
		typeName = reflectType.Name()
		typePkgPath = reflectType.PkgPath()
		if typePkgPath == "" && reflectType.Kind() == reflect.Ptr {
			typePkgPath = reflectType.Elem().PkgPath()
		}
	} else if goType != nil {
		typeName = goType.String()
		types.TypeString(goType, func(pkg *types.Package) string {
			typePkgPath = pkg.Path()
			return ""
		})
	}
	return typeName, typePkgPath
}

// getFullTypeName returns the full type name and package path for a given go/types type or reflection type.
//
// Parameters:
//   - goType: The go/types type to get the full type name and package path for
//   - reflectType: The reflection type to get the full type name and package path for
//
// Returns:
//   - string: The full type name and package path
//
// Example:
//
//	fullTypeName := getFullTypeName(goType, reflectType)
func getFullTypeName(goType types.Type, reflectType reflect.Type) string {
	typeName, typePkgPath := getTypeName(goType, reflectType)
	if typePkgPath != "" {
		return typePkgPath + "." + typeName
	}
	return typeName
}

// getFullTypeNameFromDescriptor returns the full type name and package path for a given TypeDescriptor.
//
// Parameters:
//   - desc: The TypeDescriptor to get the full type name and package path for
//
// Returns:
//   - string: The full type name and package path
//
// Example:
//
//	fullTypeName := getFullTypeNameFromDescriptor(desc)
func getFullTypeNameFromDescriptor(desc *ssztypes.TypeDescriptor) string {
	if desc.CodegenInfo != nil {
		if codegenInfo, ok := (*desc.CodegenInfo).(*CodegenInfo); ok && codegenInfo.Type != nil {
			underlyingType := codegenInfo.Type

			for {
				if ptr, ok := underlyingType.(*types.Pointer); ok {
					underlyingType = ptr.Elem()
				} else if named, ok := underlyingType.(*types.Named); ok {
					underlyingType = named.Underlying()
				} else if alias, ok := underlyingType.(*types.Alias); ok {
					underlyingType = alias.Underlying()
				} else {
					break
				}
			}

			return getFullTypeName(underlyingType, nil)
		}
	}
	return getFullTypeName(nil, desc.Type)
}
