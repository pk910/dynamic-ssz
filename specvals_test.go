// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import "testing"

// TestEvalIntSpecExpression exercises the rational spec-expression evaluator
// across every operator, the overflow/underflow/division guards, the literal
// and identifier factors, the evaluate-then-ceil-once semantics, and the
// unsupported/unresolved outcomes.
func TestEvalIntSpecExpression(t *testing.T) {
	specs := map[string]any{
		"A":   uint64(10),
		"B":   uint64(3),
		"BIG": uint64(1) << 40,
		"MAX": ^uint64(0),
		"NEG": int(-5),
		"F":   3.5, // float rounds up to 4
	}

	cases := []struct {
		name     string
		expr     string
		handled  bool
		resolved bool
		value    uint64
		wantErr  bool
	}{
		{"add", "A + B", true, true, 13, false},
		{"sub", "A - B", true, true, 7, false},
		{"mul", "A * B", true, true, 30, false},
		{"div_ceil", "A / B", true, true, 4, false},
		{"mod", "A % B", true, true, 1, false},
		{"paren", "(A + B) * B", true, true, 39, false},
		// Evaluate-then-ceil-once: an intermediate division is kept exact and
		// the whole expression is rounded up once (per-division ceil would give
		// 12 and 4 respectively).
		{"div_mul_evaluate_once", "9 / 4 * 4", true, true, 9, false},
		{"div_mul_half", "3 / 2 * 2", true, true, 3, false},
		// A chained division agrees with per-division ceil (10/3/2 -> ceil 2).
		{"chained_div", "A / B / 2", true, true, 2, false},
		// The exact value is negative -> rejected (a size cannot be negative).
		{"div_sub_negative", "1 / 2 - 1", true, false, 0, true},
		// Modulo needs integer operands; 10/3 is not integral.
		{"mod_non_integer", "A / B % 2", true, false, 0, true},
		{"literal", "42", true, true, 42, false},
		{"ident", "A", true, true, 10, false},
		{"float_ceil", "F", true, true, 4, false},
		{"add_overflow", "MAX + A", true, false, 0, true},
		{"sub_negative", "B - A", true, false, 0, true},
		{"mul_overflow", "BIG * BIG", true, false, 0, true},
		{"div_zero", "A / 0", true, false, 0, true},
		{"mod_zero", "A % 0", true, false, 0, true},
		{"literal_overflow", "99999999999999999999999", true, false, 0, true},
		{"spec_error", "NEG", true, false, 0, true},
		{"unresolved", "UNDEF", true, false, 0, false},
		{"unresolved_in_expr", "UNDEF - A", true, false, 0, false},
		{"unclosed_paren", "(A", false, false, 0, false},
		{"trailing_add_operand", "A +", false, false, 0, false},
		{"trailing_mul_operand", "A *", false, false, 0, false},
		{"paren_inner_error", "(A +", false, false, 0, false},
		{"leading_operator", "* A", false, false, 0, false},
		{"trailing_tokens", "A A", false, false, 0, false},
		{"hex_literal", "0x1F", false, false, 0, false},
		{"float_literal", "5.0", false, false, 0, false},
		{"empty", "", false, false, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handled, resolved, value, err := evalIntSpecExpression(tc.expr, specs)
			if handled != tc.handled || resolved != tc.resolved {
				t.Fatalf("evalIntSpecExpression(%q) handled=%v resolved=%v; want %v/%v (err=%v)",
					tc.expr, handled, resolved, tc.handled, tc.resolved, err)
			}
			if tc.resolved && value != tc.value {
				t.Fatalf("evalIntSpecExpression(%q) = %d; want %d", tc.expr, value, tc.value)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("evalIntSpecExpression(%q) err=%v; wantErr=%v", tc.expr, err, tc.wantErr)
			}
		})
	}
}
