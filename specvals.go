// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"

	"github.com/casbin/govaluate"
)

type cachedSpecValue struct {
	resolved bool
	value    uint64
}

// ResolveSpecValue resolves a dynamic specification value by name. The name can
// be a simple identifier (e.g., "MAX_VALIDATORS_PER_COMMITTEE") or a mathematical
// expression referencing spec values. Results are cached for subsequent lookups.
//
// Expressions restricted to the arithmetic subset (+ - * / % and parentheses
// over unsigned integer literals and spec identifiers) evaluate with exact
// uint64 arithmetic; division rounds up (ceil), since partial bytes/bits
// cannot be serialized. More complex expressions are evaluated via govaluate
// in float64 arithmetic and are rejected when the result reaches 2^53, where
// float64 can no longer represent every integer exactly.
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
	// type and full uint64 precision. govaluate evaluates everything as float64,
	// which silently loses precision near uint64 max, so it is only used for
	// actual expressions below.
	if raw, ok := d.specValues[name]; ok {
		value, resolved, err := specValueToUint64(raw)
		if err != nil {
			return false, 0, fmt.Errorf("invalid dynamic spec value %q: %w", name, err)
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

	// Expressions within the arithmetic subset evaluate with exact uint64
	// arithmetic, keeping full precision for results beyond 2^53.
	if handled, resolved, value, ierr := evalIntSpecExpression(name, d.specValues); handled {
		if ierr != nil {
			return false, 0, fmt.Errorf("invalid dynamic spec expression %q: %w", name, ierr)
		}
		cachedValue.resolved = resolved
		cachedValue.value = value

		d.specCacheMutex.Lock()
		d.specValueCache[name] = cachedValue
		d.specCacheMutex.Unlock()

		return cachedValue.resolved, cachedValue.value, nil
	}

	expression, err := govaluate.NewEvaluableExpression(name)
	if err != nil {
		return false, 0, fmt.Errorf("error parsing dynamic spec expression: %w", err)
	}

	result, err := expression.Evaluate(d.specValues)
	if err == nil {
		if value, ok := result.(float64); ok {
			// float64 represents every integer exactly only below 2^53; a
			// result at or beyond that bound may silently have lost low bits.
			if value >= math.Ldexp(1, 53) {
				return false, 0, fmt.Errorf("dynamic spec expression %q evaluates to %v, beyond the float64 integer precision range (use the integer arithmetic subset + - * / %%)", name, value)
			}
			resolved, rerr := specFloatToUint64(value)
			if rerr != nil {
				return false, 0, fmt.Errorf("invalid dynamic spec expression %q: %w", name, rerr)
			}
			cachedValue.resolved = true
			cachedValue.value = resolved
		}
	}

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
// identifiers) with exact uint64 arithmetic. Anything outside the subset
// makes the parse fail so the caller can fall back to govaluate.
type intSpecExprParser struct {
	input string
	pos   int
	specs map[string]any

	// unresolved is set when an identifier has no spec value; the expression
	// then resolves to "unknown" (static fallback) without an error, matching
	// the govaluate path's behavior for undefined references.
	unresolved bool
}

// evalIntSpecExpression evaluates expr with exact integer arithmetic.
// handled reports whether the expression is within the supported subset;
// when false, the caller must fall back to the govaluate evaluator.
// Division rounds up (ceil), since partial bytes/bits cannot be serialized.
func evalIntSpecExpression(expr string, specs map[string]any) (handled, resolved bool, value uint64, err error) {
	// Any character outside the subset alphabet means the expression uses
	// constructs (comparisons, ternaries, floats, ...) with different
	// semantics; leave those entirely to the govaluate fallback so a partial
	// arithmetic parse cannot misreport them as evaluation errors.
	for i := 0; i < len(expr); i++ {
		if !isIntExprChar(expr[i]) {
			return false, false, 0, nil
		}
	}

	p := &intSpecExprParser{input: expr, specs: specs}

	value, err = p.parseExpr()
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
	return true, true, value, nil
}

// isIntExprChar reports whether c belongs to the integer expression subset
// alphabet (identifiers, integer literals, + - * / %, parentheses, spaces).
func isIntExprChar(c byte) bool {
	return isIdentChar(c) || c == '+' || c == '-' || c == '*' || c == '/' ||
		c == '%' || c == '(' || c == ')' || c == ' ' || c == '\t'
}

// errIntExprUnsupported marks constructs outside the integer arithmetic
// subset (triggering the govaluate fallback) as opposed to genuine
// evaluation errors like overflow or division by zero.
var errIntExprUnsupported = errors.New("unsupported expression construct")

func (p *intSpecExprParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *intSpecExprParser) parseExpr() (uint64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
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
			return 0, err
		}
		switch op {
		case '+':
			sum, carry := bits.Add64(left, right, 0)
			if carry != 0 {
				return 0, fmt.Errorf("value %d + %d overflows uint64", left, right)
			}
			left = sum
		case '-':
			if right > left {
				return 0, fmt.Errorf("negative value %d - %d", left, right)
			}
			left -= right
		}
	}
}

func (p *intSpecExprParser) parseTerm() (uint64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
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
			return 0, err
		}
		switch op {
		case '*':
			hi, lo := bits.Mul64(left, right)
			if hi != 0 {
				return 0, fmt.Errorf("value %d * %d overflows uint64", left, right)
			}
			left = lo
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			// Round up: partial bytes/bits cannot be serialized, and this
			// matches the historical float-based evaluation behavior.
			left = left/right + boolToUint64(left%right != 0)
		case '%':
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			left %= right
		}
	}
}

func (p *intSpecExprParser) parseFactor() (uint64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, errIntExprUnsupported
	}

	c := p.input[p.pos]
	switch {
	case c == '(':
		p.pos++
		value, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, errIntExprUnsupported
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
			return 0, errIntExprUnsupported
		}
		value, err := strconv.ParseUint(p.input[start:p.pos], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid integer literal %q", p.input[start:p.pos])
		}
		return value, nil

	case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		start := p.pos
		for p.pos < len(p.input) && isIdentChar(p.input[p.pos]) {
			p.pos++
		}
		name := p.input[start:p.pos]
		raw, ok := p.specs[name]
		if !ok {
			p.unresolved = true
			return 0, nil
		}
		value, ok, err := specValueToUint64(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid spec value %q: %w", name, err)
		}
		if !ok {
			p.unresolved = true
			return 0, nil
		}
		return value, nil

	default:
		return 0, errIntExprUnsupported
	}
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func boolToUint64(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}
