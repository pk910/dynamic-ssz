package main

import (
	"fmt"
	"log"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	dynssz "github.com/pk910/dynamic-ssz"
)

func codegenTestCommand() {
	ds := dynssz.NewDynSsz(nil)
	ds.NoFastSsz = true

	t1 := &Test1{
		F1: phase0.ValidatorIndex(1),
		/*TestUnion: &dynssz.CompatibleUnion[struct {
			Deneb   phase0.ValidatorIndex
			Electra phase0.Epoch
		}]{
			Data:    phase0.ValidatorIndex(1),
			Variant: 0,
		},
		*/
	}
	t3 := &Test3{
		F1: 1,
		F3: 2,
		F4: 3,
	}
	t2 := &Test2{
		T1: t1,
		T3: t3,
	}

	fmt.Println("dynamic ssz:")
	root, err := ds.HashTreeRoot(t2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("root: %x\n", root)

	fmt.Println("dynamic ssz + codegen:")
	root, err = t2.HashTreeRoot()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("root: %x\n", root)

}
