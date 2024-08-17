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
		"SLOTS_PER_EPOCH":                uint64(8),
		"SYNC_COMMITTEE_SIZE":            uint64(32),
		"SYNC_COMMITTEE_SUBNET_COUNT":    uint64(4),
		"EPOCHS_PER_HISTORICAL_VECTOR":   uint64(64),
		"EPOCHS_PER_SLASHINGS_VECTOR":    uint64(64),
		"SLOTS_PER_HISTORICAL_ROOT":      uint64(64),
		"EPOCHS_PER_ETH1_VOTING_PERIOD":  uint64(4),
		"MAX_BLOB_COMMITMENTS_PER_BLOCK": uint64(16),
		"MAX_WITHDRAWALS_PER_PAYLOAD":    uint64(4),
	}

	dynssz_only_mainnet := ssz.NewDynSsz(nil)
	dynssz_only_minimal := ssz.NewDynSsz(minimalSpecs)
	//dynssz_hybrid_mainnet := ssz.NewDynSsz(nil)
	//dynssz_hybrid_minimal := ssz.NewDynSsz(minimalSpecs)

	// this has a huge negative performance impact.
	// it prevents dynssz from using fastssz for structures where no dynamic marshalling is required.
	// it's here for demonstration, don't use if not required.
	dynssz_only_mainnet.NoFastSsz = true
	dynssz_only_minimal.NoFastSsz = true

	//dynssz_hybrid_minimal.Verbose = true

	// load example blocks & states
	// these are example dumps from small kurtosis networks with mainnet & minimal presets
	// both networks were started with ~380 validators and the snapshot was made around epoch 10-20
	//block_mainnet, _ := ioutil.ReadFile("../temp/block-mainnet.ssz")
	//state_mainnet, _ := ioutil.ReadFile("../temp/state-mainnet.ssz")
	block_minimal, _ := ioutil.ReadFile("../temp/block-minimal.ssz")
	//state_minimal, _ := ioutil.ReadFile("../temp/state-minimal.ssz")

	var dur1 time.Duration
	var dur2 time.Duration
	var hash [][32]byte
	var err error
	iterations := 1

	/*
		fmt.Printf("## mainnet preset / BeaconBlock decode + encode (%d times)\n", iterations)
		dur1, dur2, hash, err = test_block_fastssz(block_mainnet, iterations)
		print_test_result("fastssz only", dur1, dur2, hash, err)
		dur1, dur2, hash, err = test_block_dynssz(dynssz_only_mainnet, block_mainnet, iterations)
		print_test_result("dynssz only", dur1, dur2, hash, err)
		//dur1, dur2, hash, err = test_block_dynssz(dynssz_hybrid_mainnet, block_mainnet, iterations)
		//print_test_result("dynssz + fastssz", dur1, dur2, hash, err)
		fmt.Printf("\n")
	*/

	/*
		fmt.Printf("## mainnet preset / BeaconState decode + encode (%d times)\n", iterations)
		dur1, dur2, hash, err = test_state_fastssz(state_mainnet, iterations)
		print_test_result("fastssz only", dur1, dur2, hash, err)
		dur1, dur2, hash, err = test_state_dynssz(dynssz_only_mainnet, state_mainnet, iterations)
		print_test_result("dynssz only", dur1, dur2, hash, err)
		dur1, dur2, hash, err = test_state_dynssz(dynssz_hybrid_mainnet, state_mainnet, iterations)
		print_test_result("dynssz + fastssz", dur1, dur2, hash, err)
		fmt.Printf("\n")
	*/

	fmt.Printf("## minimal preset / BeaconBlock decode + encode (%d times)\n", iterations)
	//dur1, dur2, hash, err = test_block_fastssz(block_minimal, iterations)
	//print_test_result("fastssz only", dur1, dur2, hash, err)
	dur1, dur2, hash, err = test_block_dynssz(dynssz_only_minimal, block_minimal, iterations)
	print_test_result("dynssz only", dur1, dur2, hash, err)
	//dur1, dur2, hash, err = test_block_dynssz(dynssz_hybrid_minimal, block_minimal, iterations)
	//print_test_result("dynssz + fastssz", dur1, dur2, hash, err)
	fmt.Printf("\n")

	/*
		fmt.Printf("## minimal preset / BeaconState decode + encode (%d times)\n", iterations)
		dur1, dur2, hash, err = test_state_fastssz(state_minimal, iterations)
		print_test_result("fastssz only", dur1, dur2, hash, err)
		dur1, dur2, hash, err = test_state_dynssz(dynssz_only_minimal, state_minimal, iterations)
		print_test_result("dynssz only", dur1, dur2, hash, err)
		dur1, dur2, hash, err = test_state_dynssz(dynssz_hybrid_minimal, state_minimal, iterations)
		print_test_result("dynssz + fastssz", dur1, dur2, hash, err)
		fmt.Printf("\n")
	*/
}

func print_test_result(title string, durationUnmarshal time.Duration, durationMarshal time.Duration, hash [][32]byte, err error) {
	fmt.Printf("%-18v", title)
	fmt.Printf("  [%4v ms / %4v ms]\t ", durationUnmarshal.Milliseconds(), durationMarshal.Milliseconds())
	if err != nil {
		fmt.Printf("failed (%v)", err)
	} else {
		fmt.Printf("success")
	}
	if len(hash) > 0 {
		fmt.Printf("\t Root: 0x%x", hash[0])
		if len(hash) > 1 && !bytes.Equal(hash[0][:], hash[1][:]) {
			fmt.Printf("\t != 0x%x", hash[1])
		}
	}

	fmt.Printf("\n")

}

func test_block_fastssz(in []byte, iterations int) (time.Duration, time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.SignedBeaconBlock)
	t.UnmarshalSSZ(in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := t.MarshalSSZ()
		if err != nil {
			return 0, 0, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZ()
	if !bytes.Equal(in, out) {
		return 0, 0, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	rootHash, _ := t.Message.HashTreeRoot()

	//out, _ = yaml.Marshal(t)
	//fmt.Printf("%v\n\n", string(out))

	return unmarshalTime, marshalTime, [][32]byte{rootHash}, nil
}

func test_state_fastssz(in []byte, iterations int) (time.Duration, time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.BeaconState)
	t.UnmarshalSSZ(in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := t.MarshalSSZ()
		if err != nil {
			return 0, 0, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZ()
	if !bytes.Equal(in, out) {
		return 0, 0, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	rootHash, _ := t.HashTreeRoot()
	return unmarshalTime, marshalTime, [][32]byte{rootHash}, nil
}

func test_block_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) (time.Duration, time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.SignedBeaconBlock)
	dynssz.UnmarshalSSZ(t, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := dynssz.MarshalSSZ(t)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := dynssz.MarshalSSZ(t)
	if !bytes.Equal(in, out) {
		return 0, 0, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	root1, _ := dynssz.HashTreeRoot(t.Message)
	fmt.Printf("  dynssz tree root: 0x%x\n", root1)

	root2, err := t.Message.HashTreeRoot()
	fmt.Printf("  fastssz tree root: 0x%x\n", root2)
	fmt.Printf("  fastssz tree err: %v\n", err)

	//out, _ = yaml.Marshal(t)
	//fmt.Printf("%v\n\n", string(out))

	return unmarshalTime, marshalTime, [][32]byte{root1, root2}, nil
}

func test_state_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) (time.Duration, time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)

		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.BeaconState)
	dynssz.UnmarshalSSZ(t, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := dynssz.MarshalSSZ(t)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := dynssz.MarshalSSZ(t)
	if !bytes.Equal(in, out) {
		return 0, 0, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	root1, _ := dynssz.HashTreeRoot(t)
	//fmt.Printf("  dynssz tree root: 0x%x\n", root1)

	root2, _ := t.HashTreeRoot()
	//fmt.Printf("  fastssz tree root: 0x%x\n", root2)

	return unmarshalTime, marshalTime, [][32]byte{root1, root2}, nil
}
