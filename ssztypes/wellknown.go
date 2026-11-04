// Copyright (c) 2026 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import "strings"

// dynsszPkgPath is the import path of the root dynamic-ssz package, home of
// the generic helper types (Union, CompatibleUnion, TypeWrapper, None).
const dynsszPkgPath = "github.com/pk910/dynamic-ssz"

// wellKnownExternalTypes is a map of external types that are known to be supported by SSZ
var wellKnownExternalTypes = map[string]SszType{
	"time.Time":                      SszUint64Type,
	"math/big.Int":                   SszBigIntType,
	"github.com/holiman/uint256.Int": SszUint256Type,
	"github.com/prysmaticlabs/go-bitfield.Bitlist": SszBitlistType,
	"github.com/OffchainLabs/go-bitfield.Bitlist":  SszBitlistType,
}

// WellKnownExternalType returns the SSZ type a well-known external type
// resolves to without a tag, or SszUnspecifiedType. The name is the type's
// declared name, with or without the type arguments reflect appends to an
// instantiated generic.
func WellKnownExternalType(pkgPath, name string) SszType {
	if t, ok := wellKnownExternalTypes[pkgPath+"."+name]; ok {
		return t
	}

	if pkgPath == dynsszPkgPath {
		if i := strings.IndexByte(name, '['); i >= 0 {
			name = name[:i]
		}
		switch name {
		case "CompatibleUnion":
			return SszCompatibleUnionType
		case "Union":
			return SszUnionType
		case "TypeWrapper":
			return SszTypeWrapperType
		}
	}

	return SszUnspecifiedType
}
