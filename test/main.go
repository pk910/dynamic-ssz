package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	ssz "github.com/pk910/dynamic-ssz"

	_ "net/http/pprof"
)

func main() {

	dynssz_mainnet := ssz.NewDynSsz(nil)
	dynssz_minimal := ssz.NewDynSsz(map[string]any{
		"SYNC_COMMITTEE_SIZE":          uint64(32),
		"SYNC_COMMITTEE_SUBNET_COUNT":  uint64(4),
		"EPOCHS_PER_HISTORICAL_VECTOR": uint64(64),
		"EPOCHS_PER_SLASHINGS_VECTOR":  uint64(64),
		"SLOTS_PER_HISTORICAL_ROOT":    uint64(64),
	})
	block_mainnet, _ := ioutil.ReadFile("block-mainnet.ssz")
	state_mainnet, _ := ioutil.ReadFile("state-mainnet.ssz")
	block_minimal, _ := ioutil.ReadFile("block-minimal.ssz")
	state_minimal, _ := ioutil.ReadFile("state-minimal.ssz")

	var dur time.Duration
	var err error

	fmt.Printf("## mainnet preset / BeaconBlock\n")
	dur, err = test_block_fastssz(block_mainnet)
	print_test_result("fastssz", dur, err)
	dur, err = test_block_dynssz(dynssz_mainnet, block_mainnet)
	print_test_result("dynssz", dur, err)
	fmt.Printf("\n")

	fmt.Printf("## mainnet preset / BeaconState\n")
	dur, err = test_state_fastssz(state_mainnet)
	print_test_result("fastssz", dur, err)
	dur, err = test_state_dynssz(dynssz_mainnet, state_mainnet)
	print_test_result("dynssz", dur, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconBlock\n")
	dur, err = test_block_fastssz(block_minimal)
	print_test_result("fastssz", dur, err)
	dur, err = test_block_dynssz(dynssz_minimal, block_minimal)
	print_test_result("dynssz", dur, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconState\n")
	dur, err = test_state_fastssz(state_minimal)
	print_test_result("fastssz", dur, err)
	dur, err = test_state_dynssz(dynssz_minimal, state_minimal)
	print_test_result("dynssz", dur, err)
	fmt.Printf("\n")

	/*
		f, _ := os.Create("mem.pprof")
		pprof.WriteHeapProfile(f)
		f.Close()
	*/
}

func print_test_result(title string, duration time.Duration, err error) {
	fmt.Printf("%v\t", title)
	fmt.Printf("[%v]\t", duration)
	if err != nil {
		fmt.Printf("failed (%v)", err)
	} else {
		fmt.Printf("success")
	}
	fmt.Printf("\n")
}

func test_block_fastssz(in []byte) (time.Duration, error) {
	start := time.Now()

	t := new(deneb.SignedBeaconBlock)
	err := t.UnmarshalSSZ(in)
	if err != nil {
		return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
	}

	out, err := t.MarshalSSZ()
	if err != nil {
		return time.Since(start), fmt.Errorf("marshal error: %v", err)
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}

func test_state_fastssz(in []byte) (time.Duration, error) {
	start := time.Now()

	t := new(deneb.BeaconState)
	err := t.UnmarshalSSZ(in)
	if err != nil {
		return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
	}

	out, err := t.MarshalSSZ()
	if err != nil {
		return time.Since(start), fmt.Errorf("marshal error: %v", err)
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}

func test_block_dynssz(dynssz *ssz.DynSsz, in []byte) (time.Duration, error) {
	start := time.Now()

	t := new(deneb.SignedBeaconBlock)
	err := dynssz.UnmarshalSSZ(t, in)
	if err != nil {
		return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
	}

	out, err := dynssz.MarshalSSZ(t)
	if err != nil {
		return time.Since(start), fmt.Errorf("marshal error: %v", err)
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}

func test_state_dynssz(dynssz *ssz.DynSsz, in []byte) (time.Duration, error) {
	start := time.Now()

	t := new(deneb.BeaconState)
	err := dynssz.UnmarshalSSZ(t, in)
	if err != nil {
		return time.Since(start), fmt.Errorf("unmarshal error: %v", err)
	}

	out, err := dynssz.MarshalSSZ(t)
	if err != nil {
		return time.Since(start), fmt.Errorf("marshal error: %v", err)
	}

	elapsed := time.Since(start)
	if !bytes.Equal(in, out) {
		return elapsed, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	return elapsed, nil
}
