//go:build ignore
// +build ignore

// Example demonstrating the new flexible code generation API
package main

import (
	"log"
	"reflect"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
)

// Example types for demonstration
type BeaconState struct {
	Slot        uint64
	BlockRoots  [][]byte `ssz-max:"8192" ssz-size:"32"`
	StateRoots  [][]byte `ssz-max:"8192" ssz-size:"32"`
	Validators  []Validator `ssz-max:"1099511627776"`
	Balances    []uint64 `ssz-max:"1099511627776"`
}

type Validator struct {
	PublicKey                  [48]byte
	WithdrawalCredentials      []byte `ssz-size:"32"`
	EffectiveBalance           uint64
	Slashed                    bool
	ActivationEligibilityEpoch uint64
	ActivationEpoch            uint64
	ExitEpoch                  uint64
	WithdrawableEpoch          uint64
}

type BeaconBlock struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	StateRoot     [32]byte
	Body          BeaconBlockBody
}

type BeaconBlockBody struct {
	RandaoReveal      [96]byte
	Eth1Data          Eth1Data
	Graffiti          [32]byte
	ProposerSlashings []ProposerSlashing `ssz-max:"16"`
	AttesterSlashings []AttesterSlashing `ssz-max:"2"`
	Attestations      []Attestation `ssz-max:"128"`
	Deposits          []Deposit `ssz-max:"16"`
	VoluntaryExits    []VoluntaryExit `ssz-max:"16"`
}

type Eth1Data struct {
	DepositRoot  [32]byte
	DepositCount uint64
	BlockHash    []byte `ssz-size:"32"`
}

type ProposerSlashing struct {
	SignedHeader1 SignedBeaconBlockHeader
	SignedHeader2 SignedBeaconBlockHeader
}

type SignedBeaconBlockHeader struct {
	Message   BeaconBlockHeader
	Signature [96]byte
}

type BeaconBlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	StateRoot     [32]byte
	BodyRoot      [32]byte
}

type AttesterSlashing struct {
	Attestation1 IndexedAttestation
	Attestation2 IndexedAttestation
}

type IndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"2048"`
	Data             AttestationData
	Signature        [96]byte
}

type Attestation struct {
	AggregationBits []byte `ssz-max:"2048"`
	Data            AttestationData
	Signature       [96]byte
}

type AttestationData struct {
	Slot            uint64
	Index           uint64
	BeaconBlockRoot [32]byte
	Source          Checkpoint
	Target          Checkpoint
}

type Checkpoint struct {
	Epoch uint64
	Root  [32]byte
}

type Deposit struct {
	Proof [][]byte `ssz-max:"33" ssz-size:"32"`
	Data  DepositData
}

type DepositData struct {
	PublicKey             [48]byte
	WithdrawalCredentials [32]byte
	Amount                uint64
	Signature             [96]byte
}

type VoluntaryExit struct {
	Epoch          uint64
	ValidatorIndex uint64
}

func main() {
	// Set up dynamic SSZ with Ethereum mainnet specs
	specs := map[string]any{
		"SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
		"EPOCHS_PER_HISTORICAL_VECTOR": uint64(65536),
		"SYNC_COMMITTEE_SIZE": uint64(512),
		"MAX_VALIDATORS_PER_COMMITTEE": uint64(2048),
	}
	
	ds := dynssz.NewDynSsz(specs)

	// Example 1: Simple convenient generation
	log.Println("Generating with ConvenientGenerate...")
	err := codegen.ConvenientGenerate(
		ds,
		"simple_generated.go",
		"main",
		reflect.TypeOf(BeaconState{}),
		reflect.TypeOf(Validator{}),
	)
	if err != nil {
		log.Fatal("ConvenientGenerate failed:", err)
	}
	log.Println("✅ Generated simple_generated.go")

	// Example 2: Advanced batch generation
	log.Println("Generating with advanced CodeGenerator...")
	generator := codegen.NewCodeGenerator(ds, &codegen.CodeGenOptions{
		CreateLegacyFn:  true,
		CreateDynamicFn: true,
	})

	// Add all Ethereum consensus types
	generator.
		AddType(reflect.TypeOf(BeaconState{}), "main").
		AddType(reflect.TypeOf(BeaconBlock{}), "main").
		AddType(reflect.TypeOf(BeaconBlockBody{}), "main").
		AddType(reflect.TypeOf(Validator{}), "main").
		AddType(reflect.TypeOf(Attestation{}), "main").
		AddType(reflect.TypeOf(AttestationData{}), "main").
		AddType(reflect.TypeOf(Deposit{}), "main").
		AddType(reflect.TypeOf(VoluntaryExit{}), "main")

	// Generate to file
	if err := generator.GenerateToFile("comprehensive_generated.go"); err != nil {
		log.Fatal("Comprehensive generation failed:", err)
	}
	log.Println("✅ Generated comprehensive_generated.go")

	// Example 3: Generate to string for inspection
	code, err := generator.GenerateToString()
	if err != nil {
		log.Fatal("String generation failed:", err)
	}

	lines := strings.Split(code, "\n")
	log.Printf("✅ Generated %d lines of code", len(lines))
	log.Printf("📊 Code size: %d characters", len(code))

	// Show first few lines as preview
	log.Println("\n📋 Generated code preview:")
	for i, line := range lines {
		if i >= 20 { // Show first 20 lines
			log.Println("... (truncated)")
			break
		}
		log.Printf("%3d: %s", i+1, line)
	}

	log.Println("\n🎉 Code generation completed successfully!")
	log.Println("Features demonstrated:")
	log.Println("  ✅ Batch collection of types to avoid duplicates")
	log.Println("  ✅ Cross-type linking and dependency resolution")
	log.Println("  ✅ Type cache checking for existing implementations")
	log.Println("  ✅ Flexible API for different generation scenarios")
	log.Println("  ✅ Automatic import management")
	log.Println("  ✅ Formatted output generation")
}