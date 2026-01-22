// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
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

func newStaticSizeVarGenerator(prefix string, typePrinter *TypePrinter, options *CodeGeneratorOptions, exprVarGenerator *exprVarGenerator) *staticSizeVarGenerator {
	return &staticSizeVarGenerator{
		prefix:           prefix,
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
	descHash := sha256.Sum256(descJson)

	if sizeVar, ok := g.varMap[descHash]; ok {
		return sizeVar, nil
	}

	g.varCounter++
	sizeVar := fmt.Sprintf("%s%d", g.prefix, g.varCounter)

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
			if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
				return "", fmt.Errorf("dynamic field not supported for static size calculation")
			} else if field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !g.options.WithoutDynamicExpressions {
				fieldSizeVar, err := g.getStaticSizeVar(field.Type)
				if err != nil {
					return "", err
				}

				fieldSizeVars = append(fieldSizeVars, fieldSizeVar)
			} else {
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
				exprVar := g.exprVarGenerator.getExprVar(*sizeExpression, uint64(desc.Len))

				if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 {
					exprVar = fmt.Sprintf("(%s+7)/8", exprVar)
				}

				appendCode(g.codeBuf, 0, "%s := %s * int(%s)\n", sizeVar, itemSizeVar, exprVar)
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
