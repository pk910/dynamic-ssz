// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"errors"
	"fmt"
	"github.com/pk910/dynamic-ssz/sszutils"
	"math"
	"math/big"
	"strconv"
)

type cachedSpecValue struct {
	resolved bool
	value    uint64
}

// ResolveSpecValue resolves a dynamic specification value by name. The name can
// be a simple identifier (e.g., "MAX_VALIDATORS_PER_COMMITTEE") or an integer
// arithmetic expression referencing spec values. Results are cached for
// subsequent lookups.
//
// Expressions support + - * / % and parentheses over unsigned integer literals
// and spec identifiers. They are evaluated as exact rationals (full precision
// across the whole uint64 range, independent of evaluation order) and rounded
// up to the next whole unit once at the end, since partial bytes/bits cannot be
// serialized. Modulo requires integer operands. A negative or out-of-range
// result, division/modulo by zero, or anything beyond this subset is an error.
//
// Returns whether the value was resolved, the uint64 value, and any parse error.
// If the name references undefined spec values, resolved will be false with no error.
func (d *DynSsz) ResolveSpecValue(name string) (bool, uint64, error) {
	d.specCacheMutex.RLock()
	cachedValue := d.specValueCache[name]
	d.specCacheMutex.RUnlock()
	if cachedValue != nil {
		return cachedValue.resolved, cachedValue.value, nil
	}

	cachedValue = &cachedSpecValue{}

	// Fast path: a spec value provided directly under this name keeps its exact
	// type and full uint64 precision without going through expression parsing. A
	// value that is present but unconvertible (negative, non-numeric, unsupported
	// type) is a misconfiguration and surfaces as an error rather than silently
	// falling back to the static limit.
	if raw, ok := d.specValues[name]; ok {
		value, resolved, err := specValueToUint64(raw)
		if err != nil {
			return false, 0, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "invalid dynamic spec value %q: %v", name, err)
		}
		if resolved {
			cachedValue.resolved = true
			cachedValue.value = value
			d.specCacheMutex.Lock()
			d.specValueCache[name] = cachedValue
			d.specCacheMutex.Unlock()
			return true, value, nil
		}
	}

	// Expressions evaluate with exact uint64 arithmetic, keeping full precision
	// across the whole value range. An undefined identifier leaves the expression
	// unresolved (the static default applies); unsupported syntax or an invalid
	// evaluation (overflow, division by zero, a present-but-invalid value) errors.
	handled, resolved, value, ierr := evalIntSpecExpression(name, d.specValues)
	if !handled {
		return false, 0, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "unsupported dynamic spec expression %q: only integer arithmetic is supported (+ - * / %%, parentheses, unsigned literals and spec identifiers)", name)
	}
	if ierr != nil {
		return false, 0, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "invalid dynamic spec expression %q: %v", name, ierr)
	}
	cachedValue.resolved = resolved
	cachedValue.value = value

	d.specCacheMutex.Lock()
	d.specValueCache[name] = cachedValue
	d.specCacheMutex.Unlock()

	return cachedValue.resolved, cachedValue.value, nil
}

// specValueToUint64 converts a directly-provided spec value to uint64, preserving
// full precision for integer types. A value stored directly under a referenced
// spec key but carrying an unsupported type (or a non-numeric string) returns an
// error so the misconfiguration surfaces instead of silently falling back to the
// static limit.
func specValueToUint64(raw any) (value uint64, ok bool, err error) {
	switch v := raw.(type) {
	case uint64:
		return v, true, nil
	case uint:
		return uint64(v), true, nil
	case uint32:
		return uint64(v), true, nil
	case uint16:
		return uint64(v), true, nil
	case uint8:
		return uint64(v), true, nil
	case uintptr:
		return uint64(v), true, nil
	case int:
		return intSpecToUint64(int64(v))
	case int64:
		return intSpecToUint64(v)
	case int32:
		return intSpecToUint64(int64(v))
	case int16:
		return intSpecToUint64(int64(v))
	case int8:
		return intSpecToUint64(int64(v))
	case float64:
		u, ferr := specFloatToUint64(v)
		return u, ferr == nil, ferr
	case float32:
		u, ferr := specFloatToUint64(float64(v))
		return u, ferr == nil, ferr
	case string:
		u, perr := strconv.ParseUint(v, 10, 64)
		if perr != nil {
			return 0, false, fmt.Errorf("string value %q is not a valid uint64", v)
		}
		return u, true, nil
	default:
		return 0, false, fmt.Errorf("unsupported type %T", raw)
	}
}

// specValueToRat converts a directly-provided spec value to an exact rational
// for use as an expression operand. Integers convert losslessly; a float keeps
// its fractional part instead of being rounded here, so a compound expression
// rounds up exactly once at the end (see ratCeilToUint64) rather than once per
// operand. Specs loaded from JSON decode as float64, so this is the common case
// for fractional values.
func specValueToRat(raw any) (*big.Rat, error) {
	switch v := raw.(type) {
	case float64:
		return floatSpecToRat(v)
	case float32:
		return floatSpecToRat(float64(v))
	default:
		// specValueToUint64 reports failure through err (ok is false only when
		// err is non-nil), so the error check alone covers every unresolvable
		// value.
		value, _, err := specValueToUint64(raw)
		if err != nil {
			return nil, err
		}
		return new(big.Rat).SetUint64(value), nil
	}
}

// floatSpecToRat converts a finite non-negative float to the exact rational it
// represents. NaN, infinity and negative values are rejected here for the same
// reasons specFloatToUint64 rejects them; the uint64 range is not checked
// because an intermediate operand may legitimately leave it (the final rounded
// result is range-checked instead).
func floatSpecToRat(v float64) (*big.Rat, error) {
	switch {
	case math.IsNaN(v):
		return nil, fmt.Errorf("value is NaN")
	case math.IsInf(v, 0):
		return nil, fmt.Errorf("value is infinite")
	case v < 0:
		return nil, fmt.Errorf("negative value %v", v)
	}

	// SetFloat64 only fails for NaN and infinity, both rejected above.
	return new(big.Rat).SetFloat64(v), nil
}

func intSpecToUint64(v int64) (uint64, bool, error) {
	if v < 0 {
		return 0, false, fmt.Errorf("negative value %d", v)
	}
	return uint64(v), true, nil
}

// specFloatToUint64 validates a float spec value and rounds it up to the next
// whole unit (partial bytes/bits cannot be serialized).
func specFloatToUint64(v float64) (uint64, error) {
	switch {
	case math.IsNaN(v):
		return 0, fmt.Errorf("value is NaN")
	case math.IsInf(v, 0):
		return 0, fmt.Errorf("value is infinite")
	case v < 0:
		return 0, fmt.Errorf("negative value %v", v)
	case v >= math.Ldexp(1, 64): // >= 2^64 overflows uint64
		return 0, fmt.Errorf("value %v overflows uint64", v)
	}
	u := uint64(v)
	if float64(u) < v {
		u++
	}
	return u, nil
}

// intSpecExprParser evaluates the arithmetic subset of spec expressions
// (+ - * / % and parentheses over unsigned integer literals and spec
// identifiers) as exact rationals; the caller rounds the result up once.
// Anything outside the subset makes the parse fail with a descriptive error.
type intSpecExprParser struct {
	input string
	pos   int
	specs map[string]any

	// unresolved is set when an identifier has no spec value; the expression
	// then resolves to "unknown" (static fallback) without an error.
	unresolved bool
}

// evalIntSpecExpression evaluates expr as an exact rational, then rounds the
// result up once. handled reports whether the expression is within the
// supported subset.
func evalIntSpecExpression(expr string, specs map[string]any) (handled, resolved bool, value uint64, err error) {
	// Any character outside the subset alphabet means the expression uses
	// unsupported constructs (comparisons, ternaries, floats, ...); reject
	// them as a whole so a partial arithmetic parse cannot misreport them
	// as evaluation errors.
	for i := 0; i < len(expr); i++ {
		if !isIntExprChar(expr[i]) {
			return false, false, 0, nil
		}
	}

	p := &intSpecExprParser{input: expr, specs: specs}

	rat, err := p.parseExpr()
	if err != nil {
		if errors.Is(err, errIntExprUnsupported) {
			return false, false, 0, nil
		}
		if p.unresolved {
			// An undefined identifier evaluates as 0 and can fabricate errors
			// (e.g. underflow); the expression is simply unresolved.
			return true, false, 0, nil
		}
		return true, false, 0, err
	}
	p.skipSpaces()
	if p.pos != len(p.input) {
		// trailing tokens outside the subset
		return false, false, 0, nil
	}
	if p.unresolved {
		return true, false, 0, nil
	}
	// The whole expression was evaluated exactly as a rational; round up once
	// to the next whole unit here.
	value, err = ratCeilToUint64(rat)
	if err != nil {
		return true, false, 0, err
	}
	return true, true, value, nil
}

// isIntExprChar reports whether c belongs to the integer expression subset
// alphabet (identifiers, integer literals, + - * / %, parentheses, spaces).
func isIntExprChar(c byte) bool {
	return isIdentChar(c) || c == '+' || c == '-' || c == '*' || c == '/' ||
		c == '%' || c == '(' || c == ')' || c == ' ' || c == '\t'
}

// errIntExprUnsupported marks constructs outside the integer arithmetic
// subset, as opposed to genuine evaluation errors like overflow or
// division by zero.
var errIntExprUnsupported = errors.New("unsupported expression construct")

func (p *intSpecExprParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *intSpecExprParser) parseExpr() (*big.Rat, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.input) {
			return left, nil
		}
		op := p.input[p.pos]
		if op != '+' && op != '-' {
			return left, nil
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		switch op {
		case '+':
			left = new(big.Rat).Add(left, right)
		case '-':
			// A negative intermediate is allowed; only the final result is
			// range-checked (a later term may bring it back non-negative).
			left = new(big.Rat).Sub(left, right)
		}
	}
}

func (p *intSpecExprParser) parseTerm() (*big.Rat, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.input) {
			return left, nil
		}
		op := p.input[p.pos]
		if op != '*' && op != '/' && op != '%' {
			return left, nil
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		switch op {
		case '*':
			left = new(big.Rat).Mul(left, right)
		case '/':
			// Division stays exact (no rounding); the whole expression is
			// rounded up once at the end. This keeps full precision across the
			// uint64 range and makes the result independent of evaluation order.
			if right.Sign() == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			left = new(big.Rat).Quo(left, right)
		case '%':
			// Modulo is only defined on integers; a non-integral operand would
			// be ambiguous, so reject it rather than guess.
			if right.Sign() == 0 {
				return nil, fmt.Errorf("modulo by zero")
			}
			if !left.IsInt() || !right.IsInt() {
				return nil, fmt.Errorf("modulo requires integer operands")
			}
			left = new(big.Rat).SetInt(new(big.Int).Mod(left.Num(), right.Num()))
		}
	}
}

func (p *intSpecExprParser) parseFactor() (*big.Rat, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, errIntExprUnsupported
	}

	c := p.input[p.pos]
	switch {
	case c == '(':
		p.pos++
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, errIntExprUnsupported
		}
		p.pos++
		return value, nil

	case c >= '0' && c <= '9':
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
		}
		// hex/underscore/float literals are outside the subset
		if p.pos < len(p.input) && (isIdentChar(p.input[p.pos]) || p.input[p.pos] == '.') {
			return nil, errIntExprUnsupported
		}
		value, err := strconv.ParseUint(p.input[start:p.pos], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer literal %q", p.input[start:p.pos])
		}
		return new(big.Rat).SetUint64(value), nil

	case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		start := p.pos
		for p.pos < len(p.input) && isIdentChar(p.input[p.pos]) {
			p.pos++
		}
		name := p.input[start:p.pos]
		raw, ok := p.specs[name]
		if !ok {
			p.unresolved = true
			return new(big.Rat), nil
		}
		value, err := specValueToRat(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid spec value %q: %w", name, err)
		}
		return value, nil

	default:
		return nil, errIntExprUnsupported
	}
}

// ratCeilToUint64 rounds a non-negative rational up to the next whole unit
// (partial bytes/bits cannot be serialized) and reports it as a uint64. A
// negative value, or one beyond the uint64 range, is an error.
func ratCeilToUint64(r *big.Rat) (uint64, error) {
	if r.Sign() < 0 {
		return 0, fmt.Errorf("expression evaluates to a negative value %s", r.RatString())
	}
	q := new(big.Int)
	m := new(big.Int)
	q.DivMod(r.Num(), r.Denom(), m) // r >= 0, so q = floor, 0 <= m < denom
	if m.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsUint64() {
		return 0, fmt.Errorf("expression value %s exceeds the uint64 range", q.String())
	}
	return q.Uint64(), nil
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
