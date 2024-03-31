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

	var dur1 time.Duration
	var dur2 time.Duration
	var err error
	iterations := 10000

	fmt.Printf("## mainnet preset / BeaconBlock decode + encode (%d times)\n", iterations)
	dur1, dur2, err = test_block_fastssz(block_mainnet, iterations)
	print_test_result("fastssz only", dur1, dur2, err)
	dur1, dur2, err = test_block_dynssz(dynssz_only_mainnet, block_mainnet, iterations)
	print_test_result("dynssz only", dur1, dur2, err)
	dur1, dur2, err = test_block_dynssz(dynssz_hybrid_mainnet, block_mainnet, iterations)
	print_test_result("dynssz + fastssz", dur1, dur2, err)
	fmt.Printf("\n")

	fmt.Printf("## mainnet preset / BeaconState decode + encode (%d times)\n", iterations)
	dur1, dur2, err = test_state_fastssz(state_mainnet, iterations)
	print_test_result("fastssz only", dur1, dur2, err)
	dur1, dur2, err = test_state_dynssz(dynssz_only_mainnet, state_mainnet, iterations)
	print_test_result("dynssz only", dur1, dur2, err)
	dur1, dur2, err = test_state_dynssz(dynssz_hybrid_mainnet, state_mainnet, iterations)
	print_test_result("dynssz + fastssz", dur1, dur2, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconBlock decode + encode (%d times)\n", iterations)
	dur1, dur2, err = test_block_fastssz(block_minimal, iterations)
	print_test_result("fastssz only", dur1, dur2, err)
	dur1, dur2, err = test_block_dynssz(dynssz_only_minimal, block_minimal, iterations)
	print_test_result("dynssz only", dur1, dur2, err)
	dur1, dur2, err = test_block_dynssz(dynssz_hybrid_minimal, block_minimal, iterations)
	print_test_result("dynssz + fastssz", dur1, dur2, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconState decode + encode (%d times)\n", iterations)
	dur1, dur2, err = test_state_fastssz(state_minimal, iterations)
	print_test_result("fastssz only", dur1, dur2, err)
	dur1, dur2, err = test_state_dynssz(dynssz_only_minimal, state_minimal, iterations)
	print_test_result("dynssz only", dur1, dur2, err)
	dur1, dur2, err = test_state_dynssz(dynssz_hybrid_minimal, state_minimal, iterations)
	print_test_result("dynssz + fastssz", dur1, dur2, err)
	fmt.Printf("\n")
}

func print_test_result(title string, durationUnmarshal time.Duration, durationMarshal time.Duration, err error) {
	fmt.Printf("%-18v", title)
	fmt.Printf("  [%4v ms / %4v ms]\t", durationUnmarshal.Milliseconds(), durationMarshal.Milliseconds())
	if err != nil {
		fmt.Printf("failed (%v)", err)
	} else {
		fmt.Printf("success")
	}
	fmt.Printf("\n")
}

func test_block_fastssz(in []byte, iterations int) (time.Duration, time.Duration, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return 0, 0, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.SignedBeaconBlock)
	t.UnmarshalSSZ(in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := t.MarshalSSZ()
		if err != nil {
			return 0, 0, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZ()
	if !bytes.Equal(in, out) {
		return 0, 0, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return unmarshalTime, marshalTime, nil
}

func test_state_fastssz(in []byte, iterations int) (time.Duration, time.Duration, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return 0, 0, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.BeaconState)
	t.UnmarshalSSZ(in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := t.MarshalSSZ()
		if err != nil {
			return 0, 0, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZ()
	if !bytes.Equal(in, out) {
		return 0, 0, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return unmarshalTime, marshalTime, nil
}

func test_block_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) (time.Duration, time.Duration, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return 0, 0, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.SignedBeaconBlock)
	dynssz.UnmarshalSSZ(t, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := dynssz.MarshalSSZ(t)
		if err != nil {
			return 0, 0, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := dynssz.MarshalSSZ(t)
	if !bytes.Equal(in, out) {
		return 0, 0, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return unmarshalTime, marshalTime, nil
}

func test_state_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) (time.Duration, time.Duration, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)

		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return 0, 0, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.BeaconState)
	dynssz.UnmarshalSSZ(t, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := dynssz.MarshalSSZ(t)
		if err != nil {
			return 0, 0, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := dynssz.MarshalSSZ(t)
	if !bytes.Equal(in, out) {
		return 0, 0, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return unmarshalTime, marshalTime, nil
}
