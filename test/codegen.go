package main

import (
	"log"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
)

type TestBeaconState deneb.BeaconState
type TestBeaconBlock deneb.SignedBeaconBlock

type Test1 struct {
	TestUnion *dynssz.CompatibleUnion[struct {
		Deneb   phase0.ValidatorIndex
		Electra phase0.Epoch
	}]
}

func codegenCommand() {
	ds := dynssz.NewDynSsz(nil)
	ds.NoFastSsz = true
	generator := codegen.NewCodeGenerator(
		ds,
		codegen.WithCreateLegacyFn(),
	)

	_, filePath, _, _ := runtime.Caller(0)
	log.Printf("Current file path: %s", filePath)
	currentDir := filepath.Dir(filePath)

	if err := generator.BuildFile(
		currentDir+"/gen_block.go",
		reflect.TypeOf(&TestBeaconBlock{}),
	); err != nil {
		log.Fatal("gen_block.go failed:", err)
	}
	if err := generator.BuildFile(
		currentDir+"/gen_state.go",
		reflect.TypeOf(&TestBeaconState{}),
	); err != nil {
		log.Fatal("gen_state.go failed:", err)
	}
	if err := generator.BuildFile(
		currentDir+"/gen_test1.go",
		reflect.TypeOf(&Test1{}),
	); err != nil {
		log.Fatal("gen_test1.go failed:", err)
	}

	err := generator.Generate()
	if err != nil {
		log.Fatal("Generation failed:", err)
	}
}
