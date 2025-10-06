// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

//go:generate dynssz-gen -package . -package-name main -legacy -types TestBeaconBlock:gen_block.go,TestSignedBeaconBlock:gen_block.go,TestBeaconState:gen_state.go,Test1:gen_test1.go,Test2:gen_test1.go,Test3:gen_test1.go

type TestBeaconState deneb.BeaconState
type TestBeaconBlock deneb.BeaconBlock

type TestSignedBeaconBlock struct {
	Message   *TestBeaconBlock
	Signature phase0.BLSSignature `ssz-size:"96"`
}

type Test1 struct {
	/*
		TestUnion *dynssz.CompatibleUnion[struct {
			Deneb   phase0.ValidatorIndex
			Electra phase0.Epoch
		}]
	*/
	F1 phase0.ValidatorIndex
}

type Test2 struct {
	T1 *Test1
	T3 *Test3 `ssz-type:"progressive-container"`
}

type Test3 struct {
	F1 uint64 `ssz-index:"1"`
	F3 uint64 `ssz-index:"3"`
	F4 uint64 `ssz-index:"4"`
}
