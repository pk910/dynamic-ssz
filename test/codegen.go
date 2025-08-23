package main

import (
	"fmt"

	"github.com/attestantio/go-eth2-client/spec/electra"
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
)

type TestType electra.BeaconState

func codegenCommand() {
	ds := dynssz.NewDynSsz(nil)
	//ds.NoFastSsz = true

	code, err := codegen.GenerateSSZCode((*TestType)(nil), codegen.WithDynSSZ(ds), codegen.WithCreateLegacyFn(), codegen.WithCreateDynamicFn())
	if err != nil {
		fmt.Printf("Error generating SSZ code: %v\n", err)
		return
	}

	fmt.Printf("%s\n", code)
}
