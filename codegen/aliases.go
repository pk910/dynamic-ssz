// Copyright (c) 2026 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/types"
	"reflect"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// AliasInfo contains information about a resolved alias type for delegation.
//
// This structure stores information about external types that can be used
// for method delegation when generating code for local type aliases.
type AliasInfo struct {
	GoType      types.Type
	ReflectType reflect.Type
	CompatFlags ssztypes.SszCompatFlag
}

type AliasInfoMap map[string]*AliasInfo

func (am *AliasInfoMap) AddGoTypesAliasFlags(aliasType types.Type, flags ssztypes.SszCompatFlag) error {
	// Get the underlying type
	underlyingType := aliasType

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

	if _, ok := underlyingType.(*types.Basic); ok {
		// do not add basic types to the alias info map
		return nil
	}

	typeKey := getFullTypeName(underlyingType, nil)
	if _, ok := (*am)[typeKey]; ok {
		return fmt.Errorf("alias for %s already registered: %s", typeKey, (*am)[typeKey].GoType.String())
	}

	(*am)[typeKey] = &AliasInfo{
		GoType:      aliasType,
		CompatFlags: flags,
	}

	return nil
}

func (am *AliasInfoMap) AddGoTypesAlias(aliasType types.Type) error {
	parser := NewParser()

	// Use the parser to check compatibility flags
	compatFlags := ssztypes.SszCompatFlag(0)

	// Check for pointer type as well
	ptrType := types.NewPointer(aliasType)

	// Check fastssz marshaler compatibility (MarshalSSZTo, SizeSSZ, UnmarshalSSZ)
	if parser.getFastsszConvertCompatibility(aliasType) || parser.getFastsszConvertCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagFastSSZMarshaler
	}

	// Check fastssz hasher compatibility (HashTreeRoot)
	if parser.getFastsszHashCompatibility(aliasType) || parser.getFastsszHashCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagFastSSZHasher
	}

	// Check HashTreeRootWith compatibility
	if parser.getHashTreeRootWithCompatibility(aliasType) || parser.getHashTreeRootWithCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagHashTreeRootWith
	}

	// Check dynamic interface compatibility
	if parser.getDynamicMarshalerCompatibility(aliasType) || parser.getDynamicMarshalerCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagDynamicMarshaler
	}
	if parser.getDynamicUnmarshalerCompatibility(aliasType) || parser.getDynamicUnmarshalerCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagDynamicUnmarshaler
	}
	if parser.getDynamicSizerCompatibility(aliasType) || parser.getDynamicSizerCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagDynamicSizer
	}
	if parser.getDynamicHashRootCompatibility(aliasType) || parser.getDynamicHashRootCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagDynamicHashRoot
	}
	if parser.getDynamicEncoderCompatibility(aliasType) || parser.getDynamicEncoderCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagDynamicEncoder
	}
	if parser.getDynamicDecoderCompatibility(aliasType) || parser.getDynamicDecoderCompatibility(ptrType) {
		compatFlags |= ssztypes.SszCompatFlagDynamicDecoder
	}

	return am.AddGoTypesAliasFlags(aliasType, compatFlags)
}

func (am *AliasInfoMap) AddReflectAliasFlags(aliasType reflect.Type, flags ssztypes.SszCompatFlag) error {
	typeKey := getFullTypeName(nil, aliasType)
	if _, ok := (*am)[typeKey]; ok {
		return fmt.Errorf("alias for %s already registered: %s", typeKey, (*am)[typeKey].ReflectType.String())
	}

	(*am)[typeKey] = &AliasInfo{
		ReflectType: aliasType,
		CompatFlags: flags,
	}
	return nil
}

func (am *AliasInfoMap) AddReflectAlias(aliasType reflect.Type) error {
	tc := ssztypes.NewTypeCache(nil)
	desc, err := tc.GetTypeDescriptor(aliasType, nil, nil, nil)
	if err != nil {
		return err
	}

	typeKey := getFullTypeName(nil, desc.Type)
	(*am)[typeKey] = &AliasInfo{
		ReflectType: aliasType,
		CompatFlags: desc.SszCompatFlags,
	}

	return nil
}
