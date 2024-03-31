package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	ssz "github.com/pk910/dynamic-ssz"
)

func main() {
	// minimal preset properties that have an effect on SSZ format
	minimalSpecs := map[string]any{
		"SYNC_COMMITTEE_SIZE":          uint64(32),
		"SYNC_COMMITTEE_SUBNET_COUNT":  uint64(4),
		"EPOCHS_PER_HISTORICAL_VECTOR": uint64(64),
		"EPOCHS_PER_SLASHINGS_VECTOR":  uint64(64),
		"SLOTS_PER_HISTORICAL_ROOT":    uint64(64),
	}

	dynssz_only_mainnet := ssz.NewDynSsz(nil)
	dynssz_only_minimal := ssz.NewDynSsz(minimalSpecs)
	dynssz_hybrid_mainnet := ssz.NewDynSsz(nil)
	dynssz_hybrid_minimal := ssz.NewDynSsz(minimalSpecs)

	// this has a huge negative performance impact.
	// it prevents dynssz from using fastssz for structures where no dynamic marshalling is required.
	// it's here for demonstration, don't use if not required.
	dynssz_only_mainnet.NoFastSsz = true
	dynssz_only_minimal.NoFastSsz = true

	// load example blocks & states
	// these are example dumps from networks with mainnet & minimal presets
	// mainnet is from the ethereum mainnet, minimal from a small kurtosis testnet
	block_mainnet, _ := ioutil.ReadFile("block-mainnet.ssz")
	state_mainnet, _ := ioutil.ReadFile("state-mainnet.ssz")
	block_minimal, _ := ioutil.ReadFile("block-minimal.ssz")
	state_minimal, _ := ioutil.ReadFile("state-minimal.ssz")

	var dur time.Duration
	var err error
	iterations := 1000

	fmt.Printf("## mainnet preset / BeaconBlock decode + encode (%d times)\n", iterations)
	dur, err = test_block_fastssz(block_mainnet, iterations)
	print_test_result("fastssz only", dur, err)
	dur, err = test_block_dynssz(dynssz_only_mainnet, block_mainnet, iterations)
	print_test_result("dynssz only", dur, err)
	dur, err = test_block_dynssz(dynssz_hybrid_mainnet, block_mainnet, iterations)
	print_test_result("dynssz + fastssz", dur, err)
	fmt.Printf("\n")

	fmt.Printf("## mainnet preset / BeaconState decode + encode (%d times)\n", iterations)
	dur, err = test_state_fastssz(state_mainnet, iterations)
	print_test_result("fastssz only", dur, err)
	dur, err = test_state_dynssz(dynssz_only_mainnet, state_mainnet, iterations)
	print_test_result("dynssz only", dur, err)
	dur, err = test_state_dynssz(dynssz_hybrid_mainnet, state_mainnet, iterations)
	print_test_result("dynssz + fastssz", dur, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconBlock decode + encode (%d times)\n", iterations)
	dur, err = test_block_fastssz(block_minimal, iterations)
	print_test_result("fastssz only", dur, err)
	dur, err = test_block_dynssz(dynssz_only_minimal, block_minimal, iterations)
	print_test_result("dynssz only", dur, err)
	dur, err = test_block_dynssz(dynssz_hybrid_minimal, block_minimal, iterations)
	print_test_result("dynssz + fastssz", dur, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconState decode + encode (%d times)\n", iterations)
	dur, err = test_state_fastssz(state_minimal, iterations)
	print_test_result("fastssz only", dur, err)
	dur, err = test_state_dynssz(dynssz_only_minimal, state_minimal, iterations)
	print_test_result("dynssz only", dur, err)
	dur, err = test_state_dynssz(dynssz_hybrid_minimal, state_minimal, iterations)
	print_test_result("dynssz + fastssz", dur, err)
	fmt.Printf("\n")
}

func print_test_result(title string, duration time.Duration, err error) {
	fmt.Printf("%-18v", title)
	fmt.Printf("  [%4v ms]\t", duration.Milliseconds())
	if err != nil {
		fmt.Printf("failed (%v)", err)
	} else {
		fmt.Printf("success")
	}
	fmt.Printf("\n")
}

func test_block_fastssz(in []byte, iterations int) (time.Duration, error) {
	start := time.Now()

	var out []byte
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
		}

		out, err = t.MarshalSSZ()
		if err != nil {
			return time.Since(start), fmt.Errorf("marshal error: %v", err)
		}
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}

func test_state_fastssz(in []byte, iterations int) (time.Duration, error) {
	start := time.Now()

	var out []byte
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
		}

		out, err = t.MarshalSSZ()
		if err != nil {
			return time.Since(start), fmt.Errorf("marshal error: %v", err)
		}
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}

func test_block_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) (time.Duration, error) {
	start := time.Now()

	var out []byte
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
		}

		out, err = dynssz.MarshalSSZ(t)
		if err != nil {
			return time.Since(start), fmt.Errorf("marshal error: %v", err)
		}
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}

func test_state_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) (time.Duration, error) {
	start := time.Now()

	var out []byte
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)
		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
		}

		out, err = dynssz.MarshalSSZ(t)
		if err != nil {
			return time.Since(start), fmt.Errorf("marshal error: %v", err)
		}
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}
