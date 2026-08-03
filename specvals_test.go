// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"
)

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
		// Fractional operands stay exact until the single final rounding; JSON
		// spec files decode every number as float64, so this is the common shape.
		"HALF": 0.5,
		"F32":  float32(1.5),
		"NAN":  math.NaN(),
		"INF":  math.Inf(1),
		"NEGF": -1.5,
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
		// A float operand must enter the arithmetic as the exact rational it is.
		// Rounding it up first turned 0.5 into 1, so these produced 4, 2 and 8.
		{"float_operand_mul", "HALF * 4", true, true, 2, false},
		{"float_operand_mul_small", "HALF * 2", true, true, 1, false},
		{"float_operand_mul_fraction", "F * 2", true, true, 7, false},
		// Two halves make a whole rather than two ceilings.
		{"float_operand_add", "HALF + HALF", true, true, 1, false},
		{"float_operand_div", "HALF / 2 * 8", true, true, 2, false},
		{"float32_operand", "F32 * 2", true, true, 3, false},
		// A lone fractional operand still rounds up, as before.
		{"float_operand_alone", "HALF", true, true, 1, false},
		{"float_nan", "NAN + 1", true, false, 0, true},
		{"float_inf", "INF + 1", true, false, 0, true},
		{"float_negative", "NEGF + 1", true, false, 0, true},
		{"add_overflow", "MAX + A", true, false, 0, true},
		{"sub_negative", "B - A", true, false, 0, true},
		{"mul_overflow", "BIG * BIG", true, false, 0, true},
		{"div_zero", "A / 0", true, false, 0, true},
		{"mod_zero", "A % 0", true, false, 0, true},
		{"literal_overflow", "99999999999999999999999", true, false, 0, true},
		{"spec_error", "NEG", true, false, 0, true},
		{"unresolved", "UNDEF", true, false, 0, false},
		{"unresolved_in_expr", "UNDEF - A", true, false, 0, false},
		// An undefined identifier evaluates as 0 and can fabricate an evaluation
		// error (here division by zero); the expression is simply unresolved,
		// not an error.
		{"unresolved_eval_error", "A / UNDEF", true, false, 0, false},
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

// Genuine evaluation errors surface wherever an undefined identifier sits in
// the expression, while a zero fabricated by the undefined identifier itself
// resolves as unknown rather than a division error.
func TestSpecExpressionErrorOrderIndependence(t *testing.T) {
	ds := NewDynSsz(map[string]any{"A": uint64(10)})

	for _, expr := range []string{"A/0+UNDEF", "UNDEF+A/0", "A%0+UNDEF", "UNDEF+A%0"} {
		if _, _, err := ds.ResolveSpecValue(expr); err == nil {
			t.Errorf("%s: genuine error swallowed", expr)
		}
	}
	for _, expr := range []string{"UNDEF+A", "A/UNDEF", "A%UNDEF", "UNDEF-A"} {
		resolved, _, err := ds.ResolveSpecValue(expr)
		if err != nil || resolved {
			t.Errorf("%s: resolved=%v err=%v, want unresolved without error", expr, resolved, err)
		}
	}
}

// Directly-supplied spec values keep their precision whatever numeric shape
// carries them.
func TestSpecValueCoercion(t *testing.T) {
	type namedU64 uint64
	big1 := new(big.Int).SetUint64(18446744073709551615)
	ds := NewDynSsz(map[string]any{
		"JNUM_INT":   json.Number("18446744073709551615"),
		"JNUM_FLOAT": json.Number("0.5"),
		"NAMED":      namedU64(42),
		"BIG":        big1,
		"BIG_NEG":    big.NewInt(-1),
	})

	expect := func(name string, want uint64) {
		resolved, got, err := ds.ResolveSpecValue(name)
		if err != nil || !resolved || got != want {
			t.Errorf("%s: resolved=%v got=%d err=%v, want %d", name, resolved, got, err, want)
		}
	}
	expect("JNUM_INT", 18446744073709551615)
	expect("NAMED", 42)
	expect("BIG", 18446744073709551615)
	// A fractional value rounds up at the end like any float spec.
	expect("JNUM_FLOAT", 1)

	if _, _, err := ds.ResolveSpecValue("BIG_NEG"); err == nil {
		t.Error("negative big.Int accepted")
	}

	type namedI32 int32
	type namedF64 float64
	type namedStruct struct{ X int }
	ds2 := NewDynSsz(map[string]any{
		"NAMED_INT":   namedI32(7),
		"NAMED_FLOAT": namedF64(2.5),
		"JNUM_BAD":    json.Number("not-a-number"),
		"STRUCT":      namedStruct{X: 1},
	})
	if resolved, got, err := ds2.ResolveSpecValue("NAMED_INT"); err != nil || !resolved || got != 7 {
		t.Errorf("NAMED_INT: resolved=%v got=%d err=%v", resolved, got, err)
	}
	if resolved, got, err := ds2.ResolveSpecValue("NAMED_FLOAT"); err != nil || !resolved || got != 3 {
		t.Errorf("NAMED_FLOAT: resolved=%v got=%d err=%v, want the once-ceiled 3", resolved, got, err)
	}
	if _, _, err := ds2.ResolveSpecValue("JNUM_BAD"); err == nil {
		t.Error("malformed json.Number accepted")
	}
	if _, _, err := ds2.ResolveSpecValue("STRUCT"); err == nil {
		t.Error("struct-typed spec value accepted")
	}
}

// A spec key that spells an expression answers the lookup directly, with the
// blanks the expression grammar ignores not counting against the match.
func TestSpecDirectKeyWhitespace(t *testing.T) {
	ds := NewDynSsz(map[string]any{"A+B": uint64(99), "A": uint64(10), "B": uint64(3)})
	for _, name := range []string{"A+B", "A + B", "A +\tB"} {
		resolved, got, err := ds.ResolveSpecValue(name)
		if err != nil || !resolved || got != 99 {
			t.Errorf("%q: resolved=%v got=%d err=%v, want the direct 99", name, resolved, got, err)
		}
	}
}
