package main

import (
	"fmt"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
)

type TestBeaconState deneb.BeaconState
type TestBeaconBlock struct {
	Message   *deneb.BeaconBlock
	Signature phase0.BLSSignature `ssz-size:"96"`
}

type Test1 struct {
	TestUnion dynssz.CompatibleUnion[struct {
		Deneb   phase0.ValidatorIndex
		Electra phase0.Epoch
	}]
}

func codegenCommand() {
	ds := dynssz.NewDynSsz(nil)
	ds.NoFastSsz = true

	code, err := codegen.GenerateSSZCode((*Test1)(nil), codegen.WithDynSSZ(ds), codegen.WithCreateLegacyFn(), codegen.WithCreateDynamicFn())
	if err != nil {
		fmt.Printf("Error generating SSZ code: %v\n", err)
		return
	}

	fmt.Printf("%s\n", code)
}
