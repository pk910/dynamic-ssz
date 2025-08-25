package main

import (
	"fmt"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
)

type TestBeaconState deneb.BeaconState
type TestBeaconBlock deneb.SignedBeaconBlock

func codegenCommand() {
	ds := dynssz.NewDynSsz(nil)
	ds.NoFastSsz = true

	code, err := codegen.GenerateSSZCode((*TestBeaconBlock)(nil), codegen.WithDynSSZ(ds), codegen.WithCreateLegacyFn(), codegen.WithCreateDynamicFn())
	if err != nil {
		fmt.Printf("Error generating SSZ code: %v\n", err)
		return
	}

	fmt.Printf("%s\n", code)
}
