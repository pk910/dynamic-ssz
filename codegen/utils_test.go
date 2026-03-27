// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"strings"
	"testing"
)

// TestWriteIndentedTrailingContent tests writeIndented with content that does not end in newline.
func TestWriteIndentedTrailingContent(t *testing.T) {
	var b strings.Builder
	writeIndented(&b, "hello", 1)
	if b.String() != "\thello" {
		t.Errorf("expected '\\thello', got %q", b.String())
	}
}

// TestWriteIndentedMultiLine tests writeIndented with multiple lines including trailing content.
func TestWriteIndentedMultiLine(t *testing.T) {
	var b strings.Builder
	writeIndented(&b, "a\nb\nc", 1)
	expected := "\ta\n\tb\n\tc"
	if b.String() != expected {
		t.Errorf("expected %q, got %q", expected, b.String())
	}
}

// TestEscapeBackticks tests escapeBackticks with and without backticks.
func TestEscapeBackticks(t *testing.T) {
	// Without backticks - should return unchanged
	result := escapeBackticks("hello world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}

	// With backticks - strconv.Quote keeps them as-is in double-quoted strings,
	// but the function still processes them through Quote to handle any other special chars.
	input := "hello \x60world\x60"
	result = escapeBackticks(input)
	if !strings.Contains(result, "world") {
		t.Errorf("expected result containing 'world', got %q", result)
	}

	// With special chars alongside backticks
	input = "test\x60\ttab\x60"
	result = escapeBackticks(input)
	if !strings.Contains(result, "\\t") {
		t.Errorf("expected tab to be escaped, got %q", result)
	}
}

// TestIndentStr tests indentStr with various inputs.
func TestIndentStr(t *testing.T) {
	// Zero indent
	result := indentStr("hello", 0)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}

	// Empty string
	result = indentStr("", 1)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}

	// String without trailing newline
	result = indentStr("hello", 1)
	if result != "\thello" {
		t.Errorf("expected '\\thello', got %q", result)
	}
}
