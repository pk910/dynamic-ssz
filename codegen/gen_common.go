// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

const (
	varNameVLen     = "vlen"
	varNameElemSize = "elemSize"
)

// convSuppressComment rides emitted lines that convert a resolved ctx.exprs
// value to int. The value is bounded by the platform-size guard emitted at
// resolution, but CodeQL and gosec do not carry range facts through array
// elements, so the exact conversion is flagged. gosec requires #nosec at the
// comment start; the lgtm marker is CodeQL's end-of-line suppression form
// (codeql[...] only works as a standalone line), matched anywhere in the
// comment, and takes effect in scans that include the alert-suppression
// queries.
const convSuppressComment = " // #nosec G115 lgtm[go/incorrect-integer-conversion] -- bounded by the platform-size guard at spec resolution"

// Generated error expression constants shared across codegen files.
const (
	errCodeBitvectorPadding       = "sszutils.ErrBitvectorPaddingFn()"
	errCodeUnionTypeMismatch      = "sszutils.ErrUnionTypeMismatchFn()"
	errCodeInvalidUnionVariant    = "sszutils.ErrInvalidUnionVariantFn()"
	errCodeBitlistNotTerminated   = "sszutils.ErrBitlistNotTerminatedFn()"
	errCodeCustomTypeNotSupported = "sszutils.ErrCustomTypeNotSupportedFn()"
	errCodeTrailingData           = "sszutils.ErrTrailingDataFn(diff)"
)

type exprVarGenerator struct {
	prefix      string
	typePrinter *TypePrinter
	options     *CodeGeneratorOptions
	isSlice     bool
	retVars     string
	codeBuf     *strings.Builder
	varMap      map[[32]byte]string
	varCounter  int
}

func newExprVarGenerator(prefix string, typePrinter *TypePrinter, options *CodeGeneratorOptions) *exprVarGenerator {
	return &exprVarGenerator{
		prefix:      prefix,
		typePrinter: typePrinter,
		options:     options,
		retVars:     "err",
		codeBuf:     &strings.Builder{},
		varMap:      make(map[[32]byte]string),
		varCounter:  0,
	}
}

// getExprVar generates a variable name for cached limit expression calculations.
func (g *exprVarGenerator) getExprVar(expr string, defaultValue uint64) string {
	if expr == "" {
		return fmt.Sprintf("%v", defaultValue)
	}

	exprKey := sha256.Sum256([]byte(fmt.Sprintf("%s\n%v", expr, defaultValue)))
	if exprVar, ok := g.varMap[exprKey]; ok {
		return exprVar
	}

	varNamePattern := "%s%d"
	varDefColon := ":"
	if g.isSlice {
		varNamePattern = "%s[%d]"
		varDefColon = ""
	}
	exprVar := fmt.Sprintf(varNamePattern, g.prefix, g.varCounter)
	g.varCounter++

	appendCode(g.codeBuf, 0, "%s, err %s= sszutils.ResolveSpecValueWithDefault(ds, \"%s\", %d)\n", exprVar, varDefColon, expr, defaultValue)
	appendCode(g.codeBuf, 0, "if err != nil {\n")
	appendCode(g.codeBuf, 1, "return %s\n", g.retVars)
	appendCode(g.codeBuf, 0, "}\n")

	g.varMap[exprKey] = exprVar

	return exprVar
}

// getSizeExprVar resolves a size-domain spec expression (a vector or byte
// size, as opposed to a list limit) via getExprVar and additionally rejects a
// resolved value above the platform integer range: sizes pass through int at
// their use sites (allocations, loop bounds, the int-based codec surface), so
// the guard makes those conversions exact. List limits keep their full uint64
// range by resolving through getExprVar directly.
func (g *exprVarGenerator) getSizeExprVar(expr string, defaultValue uint64) string {
	if expr == "" {
		return fmt.Sprintf("%v", defaultValue)
	}

	exprVar := g.getExprVar(expr, defaultValue)

	guardKey := sha256.Sum256([]byte(fmt.Sprintf("sizeguard\n%s\n%v", expr, defaultValue)))
	if _, ok := g.varMap[guardKey]; ok {
		return exprVar
	}

	mathPkgName := g.typePrinter.AddImport("math", "math")
	appendCode(g.codeBuf, 0, "if %s > %s.MaxInt {\n", exprVar, mathPkgName)
	appendCode(g.codeBuf, 1, "err = sszutils.ErrPlatformOverflowFn(\"size expression %s\", %s)\n", expr, exprVar)
	appendCode(g.codeBuf, 1, "return %s\n", g.retVars)
	appendCode(g.codeBuf, 0, "}\n")

	g.varMap[guardKey] = exprVar
	return exprVar
}

func (g *exprVarGenerator) withRetVars(retVars string) func() {
	oldRetVars := g.retVars
	g.retVars = retVars
	return func() {
		g.retVars = oldRetVars
	}
}

func (g *exprVarGenerator) getCode() string {
	return g.codeBuf.String()
}

type staticSizeVarGenerator struct {
	prefix           string
	typePrinter      *TypePrinter
	options          *CodeGeneratorOptions
	exprVarGenerator *exprVarGenerator
	codeBuf          *strings.Builder
	varMap           map[[32]byte]string
	varCounter       int
}

func newStaticSizeVarGenerator(typePrinter *TypePrinter, options *CodeGeneratorOptions, exprVarGenerator *exprVarGenerator) *staticSizeVarGenerator {
	return &staticSizeVarGenerator{
		prefix:           "size",
		typePrinter:      typePrinter,
		options:          options,
		exprVarGenerator: exprVarGenerator,
		codeBuf:          &strings.Builder{},
		varMap:           make(map[[32]byte]string),
		varCounter:       0,
	}
}

// getStaticSizeVar generates a variable name for cached static size calculations.
func (g *staticSizeVarGenerator) getStaticSizeVar(desc *ssztypes.TypeDescriptor) (string, error) {
	descCopy := *desc
	descCopy.GoTypeFlags &= ^ssztypes.GoTypeFlagIsPointer

	descJson, err := json.Marshal(descCopy)
	if err != nil {
		return "", err
	}
	// The JSON alone cannot distinguish shallow-built delegated descriptors
	// (they carry no kind/size/subtree), so include the Go type identity in
	// the dedup key; otherwise two different delegated types would share one
	// size variable.
	descJson = append(descJson, g.typePrinter.TypeStringWithoutTracking(desc, false)...)
	descHash := sha256.Sum256(descJson)

	if sizeVar, ok := g.varMap[descHash]; ok {
		return sizeVar, nil
	}

	g.varCounter++
	sizeVar := fmt.Sprintf("%s%d", g.prefix, g.varCounter)

	// A shallow-built, fully-delegated type (parser gate) has no traversed subtree
	// to sum: it computes its own (possibly spec-dependent) fixed size, so query
	// the type's sizer on a zero value at runtime. The gate only admits types that
	// implement DynamicSizer (fullyDelegatesSSZ requires it), so that is the only
	// case to handle here.
	if desc.SszType == ssztypes.SszUnspecifiedType && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 {
		// The sizer speaks int; clamping a misbehaving negative to zero keeps
		// the unsigned sum from wrapping huge.
		appendCode(g.codeBuf, 0, "%s := uint64(sszutils.Max(new(%s).SizeSSZDyn(ds), 0))\n", sizeVar, g.typePrinter.InnerTypeString(desc))
		g.varMap[descHash] = sizeVar
		return sizeVar, nil
	}

	// recursive resolve static size with size expressions
	switch desc.SszType {
	case ssztypes.SszTypeWrapperType:
		sizeVar, err = g.getStaticSizeVar(desc.ElemDesc)
		if err != nil {
			return "", err
		}
	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		fieldSizeVars := []string{}
		staticSize := 0
		for _, field := range desc.ContainerDesc.Fields {
			var fieldSizeVar string
			switch {
			case field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0:
				return "", fmt.Errorf("dynamic field not supported for static size calculation")
			case field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !g.options.WithoutDynamicExpressions:
				fieldSizeVar, err = g.getStaticSizeVar(field.Type)
				if err != nil {
					return "", err
				}

				fieldSizeVars = append(fieldSizeVars, fieldSizeVar)
			default:
				staticSize += int(field.Type.Size)
			}
		}

		fieldSizeVars = append(fieldSizeVars, fmt.Sprintf("%d", staticSize))
		if len(fieldSizeVars) == 1 {
			return fieldSizeVars[0], nil
		}
		appendCode(g.codeBuf, 0, "%s := %s // size expression for '%s'\n", sizeVar, strings.Join(fieldSizeVars, " + "), g.typePrinter.TypeStringWithoutTracking(desc, false))
	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		sizeExpression := desc.SizeExpression
		if g.options.WithoutDynamicExpressions {
			sizeExpression = nil
		}

		if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			return "", fmt.Errorf("dynamic vector not supported for static size calculation")
		} else {
			itemSizeVar := ""
			if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !g.options.WithoutDynamicExpressions {
				itemSizeVar, err = g.getStaticSizeVar(desc.ElemDesc)
				if err != nil {
					return "", err
				}
			} else {
				itemSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
			}

			if sizeExpression != nil {
				// Bit-size expressions resolve to a bit count, so their default
				// must be the bit size (matching the marshal/unmarshal paths),
				// not the byte length.
				defaultValue := uint64(desc.Len)
				if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.BitSize > 0 {
					defaultValue = uint64(desc.BitSize)
				}
				exprVar := g.exprVarGenerator.getSizeExprVar(*sizeExpression, defaultValue)

				if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 {
					exprVar = fmt.Sprintf("(%s+7)/8", exprVar)
				}

				appendCode(g.codeBuf, 0, "%s := %s * %s\n", sizeVar, itemSizeVar, exprVar)
			} else if _, lerr := strconv.ParseUint(itemSizeVar, 10, 64); lerr == nil {
				// A fully literal product needs the explicit uint64 type to
				// join the other unsigned size variables.
				appendCode(g.codeBuf, 0, "%s := uint64(%s * %d)\n", sizeVar, itemSizeVar, desc.Len)
			} else {
				appendCode(g.codeBuf, 0, "%s := %s * %d\n", sizeVar, itemSizeVar, desc.Len)
			}
		}

	default:
		return "", fmt.Errorf("unknown type for static size calculation: %v", desc.SszType)
	}

	g.varMap[descHash] = sizeVar

	return sizeVar, nil
}

func (g *staticSizeVarGenerator) getCode() string {
	return g.codeBuf.String()
}

type typePathList []typePathSegment

type typePathSegment struct {
	tpl  string
	args []string
}

func (tp typePathList) append(tpl string, args ...string) typePathList {
	segment := typePathSegment{
		tpl:  tpl,
		args: args,
	}
	return append(tp, segment)
}

func (tp typePathList) getErrorWithArgs() (string, int) {
	pathFormat := strings.Builder{}
	pathArgs := []string{}
	for idx, segment := range tp {
		if idx > 0 && !strings.HasPrefix(segment.tpl, "[") {
			pathFormat.WriteString(".")
		}
		pathFormat.WriteString(segment.tpl)
		pathArgs = append(pathArgs, segment.args...)
	}

	if len(pathArgs) == 0 {
		return fmt.Sprintf("%q", pathFormat.String()), 0
	}

	return fmt.Sprintf("%q, %s", pathFormat.String(), strings.Join(pathArgs, ", ")), len(pathArgs)
}

func (tp typePathList) getErrorWith(err string) string {
	errArgs, argCount := tp.getErrorWithArgs()
	if len(errArgs) > 2 {
		if argCount > 0 {
			return fmt.Sprintf("sszutils.ErrorWithPathf(%s, %s)", err, errArgs)
		}
		return fmt.Sprintf("sszutils.ErrorWithPath(%s, %s)", err, errArgs)
	}
	return err
}

// minSizeExpr generates the code-generation counterpart of ssztypes.MinSize:
// an expression for the smallest a value of desc can serialize to. It returns
// the expression, a sub-expression the caller must check is non-zero before
// dividing by the result (empty when the value is always positive), and false
// when no bound can be stated.
//
// A spec-driven size must not be baked in as a constant. The generator sees the
// static tag values while the caller may run a preset that resolves them
// smaller, so those parts resolve to the same runtime size variables the rest of
// the generated method uses -- the constant and the expression would otherwise
// disagree about the same bytes. WithoutDynamicExpressions freezes every size to
// what the generator saw, so there everything folds back to constants.
func minSizeExpr(desc *ssztypes.TypeDescriptor, sizeVars *staticSizeVarGenerator, options *CodeGeneratorOptions) (expr, positiveGuard string, ok bool) {
	if desc == nil {
		return "", "", false
	}

	// A static type serializes to exactly its size, which is a constant unless a
	// spec value feeds it.
	if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr == 0 || options.WithoutDynamicExpressions {
			return fmt.Sprintf("%d", desc.Size), "", desc.Size > 0
		}

		sizeVar, err := sizeVars.getStaticSizeVar(desc)
		if err != nil {
			return "", "", false
		}

		return sizeVar, sizeVar, true
	}

	switch desc.SszType {
	case ssztypes.SszTypeWrapperType:
		return minSizeExpr(desc.ElemDesc, sizeVars, options)

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		staticSize := 0
		sizeParts := []string{}
		for _, field := range desc.ContainerDesc.Fields {
			switch {
			case field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0:
				staticSize += 4
			case field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions:
				sizeVar, err := sizeVars.getStaticSizeVar(field.Type)
				if err != nil {
					return "", "", false
				}
				sizeParts = append(sizeParts, sizeVar)
			default:
				staticSize += int(field.Type.Size)
			}
		}

		if len(sizeParts) == 0 {
			return fmt.Sprintf("%d", staticSize), "", staticSize > 0
		}

		// A dynamic container has at least one dynamic field, so the constant
		// term holds that field's 4 offset bytes and the sum is never zero.
		sizeParts = append(sizeParts, fmt.Sprintf("%d", staticSize))

		return strings.Join(sizeParts, "+"), "", true

	case ssztypes.SszVectorType:
		// A vector of dynamic elements leads with one 4-byte offset per element,
		// and every element costs at least its own minimum on top of that.
		elemMin, _, elemOk := minSizeExpr(desc.ElemDesc, sizeVars, options)
		if !elemOk {
			elemMin = "0"
		}
		perElem, perElemOk := mulOrAddExpr("+", "4", elemMin)
		if !perElemOk {
			return "", "", false
		}

		// desc.Len is the element count here, not a byte size, and a count that
		// comes from a spec value can resolve to zero however the tag reads.
		if desc.SizeExpression == nil || options.WithoutDynamicExpressions {
			count := fmt.Sprintf("%d", desc.Len)
			expr, exprOk := mulOrAddExpr("*", count, perElem)

			return expr, "", exprOk && desc.Len > 0
		}

		count := sizeVars.exprVarGenerator.getSizeExprVar(*desc.SizeExpression, uint64(desc.Len))
		expr, exprOk := mulOrAddExpr("*", count, perElem)

		// The product is what the caller divides by, so it is what has to be
		// checked: a spec value large enough to overflow the multiplication
		// wraps to zero or a small value, neither of which bounds anything. The
		// reflection engine drops such a product for the same reason.
		return expr, expr, exprOk

	default:
		return "", "", false
	}
}

// mulOrAddExpr joins two size expressions, folding them when both are literals
// so a fully static bound stays a plain number in the generated code. It reports
// false if the result would be zero, which states no bound.
func mulOrAddExpr(op, left, right string) (string, bool) {
	leftVal, leftErr := strconv.Atoi(left)
	rightVal, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		folded := leftVal * rightVal
		if op == "+" {
			folded = leftVal + rightVal
		}

		return fmt.Sprintf("%d", folded), folded > 0
	}

	if op == "+" && right == "0" {
		return left, true
	}

	// Only a compound operand needs grouping; wrapping a bare number or variable
	// just makes the generated expression harder to read.
	if strings.ContainsAny(right, "+-*/") {
		right = "(" + right + ")"
	}

	return left + op + right, true
}

// indexBase parenthesizes a value expression so it can be used as an indexing
// or slicing base. A dereferenced pointer receiver (e.g. "*t") must become
// "(*t)" before "[i]" so it binds as (*t)[i] rather than *(t[i]).
func indexBase(valueVar string) string {
	if strings.HasPrefix(valueVar, "*") {
		return "(" + valueVar + ")"
	}
	return valueVar
}

// localizedVarName returns the name to bind a localized value to. The generated
// method/closure already declares "t" (its receiver or parameter), and a
// pointer nil-check localizes to "t" as well. A value rooted in a localized
// name must not redeclare that name — TypeWrapper recursion stays in the same
// brace scope — so the root identifier of the incoming value advances a
// numeric suffix instead: t -> t2 -> t3. An unrelated root shadows "t" inside
// its own block, except at the top scope where "t" is the receiver.
func localizedVarName(varName string, indent int) string {
	root := varName
	for i, r := range varName {
		if r == '.' || r == '[' {
			root = varName[:i]
			break
		}
	}
	root = strings.TrimSuffix(strings.TrimLeft(root, "(*"), ")")

	if rest, ok := strings.CutPrefix(root, "t"); ok {
		if rest == "" {
			return "t2"
		}
		if n, err := strconv.Atoi(rest); err == nil {
			return fmt.Sprintf("t%d", n+1)
		}
	}
	if indent == 0 {
		return "t2"
	}
	return "t"
}

// intCapExpr returns expr usable as an int capacity cap. A capacity above the
// platform integer range clamps to math.MaxInt — the limit check itself
// compares in uint64, so nothing is lost — while a plain conversion of a
// 64-bit limit could wrap negative. Integer literals are resolved at
// generation time: small ones pass through untouched, larger ones emit the
// runtime clamp so the generated code compiles on 32-bit platforms too. The
// qualified helper keeps the generated code clear of the bare min builtin,
// which a target package may shadow.
func intCapExpr(expr string) string {
	if v, err := strconv.ParseUint(expr, 10, 64); err == nil && v <= math.MaxInt32 {
		return expr
	}
	return fmt.Sprintf("sszutils.CapToInt(%s)", expr)
}

// uintCmpExpr renders a length comparison against a resolved size or limit.
// Expression values compare in uint64 (lengths are never negative, and the
// resolved value spans the full range); literals in the portable int range
// compare as plain int constants, sparing the pointless cast.
func uintCmpExpr(lenExpr, op, limit string) string {
	if v, err := strconv.ParseUint(limit, 10, 64); err == nil && v <= math.MaxInt32 {
		return fmt.Sprintf("%s %s %s", lenExpr, op, limit)
	}
	return fmt.Sprintf("uint64(%s) %s %s", lenExpr, op, limit)
}

// uintLitArg types an integer literal above the portable int range as uint64
// for splicing into an `any` argument (error constructors). An untyped
// constant there defaults to int, which does not compile for values past the
// target's int width. Variables and smaller literals pass through untouched.
func uintLitArg(expr string) string {
	if v, err := strconv.ParseUint(expr, 10, 64); err == nil && v > math.MaxInt32 {
		return fmt.Sprintf("uint64(%s)", expr)
	}
	return expr
}
