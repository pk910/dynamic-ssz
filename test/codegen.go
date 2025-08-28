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

type Test2 struct {
	T1 *Test1
}

func codegenCommand() {
	ds := dynssz.NewDynSsz(nil)
	//ds.NoFastSsz = true
	generator := codegen.NewCodeGenerator(ds)

	_, filePath, _, _ := runtime.Caller(0)
	log.Printf("Current file path: %s", filePath)
	currentDir := filepath.Dir(filePath)

	generator.BuildFile(
		currentDir+"/gen_block.go",
		codegen.WithType(
			reflect.TypeOf(&TestBeaconBlock{}),
			codegen.WithCreateLegacyFn(),
		),
	)

	generator.BuildFile(
		currentDir+"/gen_state.go",
		codegen.WithType(
			reflect.TypeOf(&TestBeaconState{}),
			codegen.WithCreateLegacyFn(),
		),
	)

	generator.BuildFile(
		currentDir+"/gen_test1.go",
		codegen.WithType(
			reflect.TypeOf(&Test1{}),
		),
		codegen.WithType(
			reflect.TypeOf(&Test2{}),
		),
		codegen.WithCreateLegacyFn(),
	)

	err := generator.Generate()
	if err != nil {
		log.Fatal("Generation failed:", err)
	}
}
