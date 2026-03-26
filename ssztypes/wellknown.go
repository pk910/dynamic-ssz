// Copyright (c) 2026 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import "strings"

// wellKnownExternalTypes is a map of external types that are known to be supported by SSZ
var wellKnownExternalTypes = map[string]SszType{
	"time.Time":                      SszUint64Type,
	"math/big.Int":                   SszBigIntType,
	"github.com/holiman/uint256.Int": SszUint256Type,
	"github.com/prysmaticlabs/go-bitfield.Bitlist": SszBitlistType,
	"github.com/OffchainLabs/go-bitfield.Bitlist":  SszBitlistType,
}

// getWellKnownExternalType returns the SSZ type for a well-known external type
func getWellKnownExternalType(pkgPath, name string) SszType {
	if t, ok := wellKnownExternalTypes[pkgPath+"."+name]; ok {
		return t
	}

	if pkgPath == "github.com/pk910/dynamic-ssz" {
		if strings.HasPrefix(name, "CompatibleUnion[") {
			return SszCompatibleUnionType
		}
		if strings.HasPrefix(name, "TypeWrapper[") {
			return SszTypeWrapperType
		}
	}

	return SszUnspecifiedType
}
