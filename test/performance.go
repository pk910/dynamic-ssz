package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	ssz "github.com/pk910/dynamic-ssz"
	"gopkg.in/yaml.v2"
)

func performanceCommand() {
	minimalPresetBytes, _ := ioutil.ReadFile("minimal-preset.yaml")
	minimalSpecs := make(map[string]any)
	yaml.Unmarshal(minimalPresetBytes, &minimalSpecs)

	dynssz_only_mainnet := ssz.NewDynSsz(nil)
	dynssz_only_minimal := ssz.NewDynSsz(minimalSpecs)
	dynssz_hybrid_mainnet := ssz.NewDynSsz(nil)
	dynssz_hybrid_minimal := ssz.NewDynSsz(minimalSpecs)

	// this has a huge negative performance impact.
	// it prevents dynssz from using fastssz for structures where no dynamic marshalling is required.
	// it's here for demonstration, don't use if not required.
	dynssz_only_mainnet.NoFastSsz = true
	dynssz_only_minimal.NoFastSsz = true

	//dynssz_hybrid_minimal.Verbose = true

	// load example blocks & states
	// these are example dumps from small kurtosis networks with mainnet & minimal presets
	// both networks were started with ~380 validators and the snapshot was made around epoch 10-20
	block_mainnet, _ := ioutil.ReadFile("../temp/block-mainnet.ssz")
	state_mainnet, _ := ioutil.ReadFile("../temp/state-mainnet.ssz")
	block_minimal, _ := ioutil.ReadFile("../temp/block-minimal.ssz")
	state_minimal, _ := ioutil.ReadFile("../temp/state-minimal.ssz")

	var dur []time.Duration
	var hash [][32]byte
	var err error
	iterations := 1000

	fmt.Printf("## mainnet preset / BeaconBlock decode + encode + hash (%d times)\n", iterations)
	dur, hash, err = test_block_fastssz(block_mainnet, iterations)
	print_test_result("fastssz only", dur, hash, err)
	dur, hash, err = test_block_dynssz(dynssz_only_mainnet, block_mainnet, iterations)
	print_test_result("dynssz only", dur, hash, err)
	dur, hash, err = test_block_dynssz(dynssz_hybrid_mainnet, block_mainnet, iterations)
	print_test_result("dynssz + fastssz", dur, hash, err)
	dur, hash, err = test_block_dynssz_codegen(dynssz_hybrid_mainnet, block_mainnet, iterations)
	print_test_result("dynssz + codegen", dur, hash, err)
	fmt.Printf("\n")

	fmt.Printf("## mainnet preset / BeaconState decode + encode + hash (%d times)\n", iterations)
	dur, hash, err = test_state_fastssz(state_mainnet, iterations)
	print_test_result("fastssz only", dur, hash, err)
	dur, hash, err = test_state_dynssz_codegen(dynssz_hybrid_mainnet, state_mainnet, iterations)
	print_test_result("dynssz + codegen", dur, hash, err)
	dur, hash, err = test_state_dynssz(dynssz_only_mainnet, state_mainnet, iterations)
	print_test_result("dynssz only", dur, hash, err)
	dur, hash, err = test_state_dynssz(dynssz_hybrid_mainnet, state_mainnet, iterations)
	print_test_result("dynssz + fastssz", dur, hash, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconBlock decode + encode + hash (%d times)\n", iterations)
	dur, hash, err = test_block_fastssz(block_minimal, iterations)
	print_test_result("fastssz only", dur, hash, err)
	dur, hash, err = test_block_dynssz(dynssz_only_minimal, block_minimal, iterations)
	print_test_result("dynssz only", dur, hash, err)
	dur, hash, err = test_block_dynssz(dynssz_hybrid_minimal, block_minimal, iterations)
	print_test_result("dynssz + fastssz", dur, hash, err)
	dur, hash, err = test_block_dynssz_codegen(dynssz_hybrid_minimal, block_minimal, iterations)
	print_test_result("dynssz + codegen", dur, hash, err)
	fmt.Printf("\n")

	fmt.Printf("## minimal preset / BeaconState decode + encode + hash (%d times)\n", iterations)
	dur, hash, err = test_state_fastssz(state_minimal, iterations)
	print_test_result("fastssz only", dur, hash, err)
	dur, hash, err = test_state_dynssz(dynssz_only_minimal, state_minimal, iterations)
	print_test_result("dynssz only", dur, hash, err)
	dur, hash, err = test_state_dynssz(dynssz_hybrid_minimal, state_minimal, iterations)
	print_test_result("dynssz + fastssz", dur, hash, err)
	dur, hash, err = test_state_dynssz_codegen(dynssz_hybrid_minimal, state_minimal, iterations)
	print_test_result("dynssz + codegen", dur, hash, err)
	fmt.Printf("\n")
}

func print_test_result(title string, durations []time.Duration, hash [][32]byte, err error) {
	fmt.Printf("%-18v  ", title)
	if len(durations) > 0 {
		fmt.Printf("[%4v ms / %4v ms / %4v ms]", durations[0].Milliseconds(), durations[1].Milliseconds(), durations[2].Milliseconds())
	} else {
		fmt.Printf("[    ms /     ms /     ms]")
	}
	fmt.Printf("\t ")
	if err != nil {
		fmt.Printf("failed (%v)", err)
	} else {
		fmt.Printf("success")
	}
	if len(hash) > 0 {
		fmt.Printf("\t Root: 0x%x", hash[0])
	}

	fmt.Printf("\n")
}

func test_block_fastssz(in []byte, iterations int) ([]time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)
	hashTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.SignedBeaconBlock)
	t.UnmarshalSSZ(in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := t.MarshalSSZ()
		if err != nil {
			return nil, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZ()
	if !bytes.Equal(in, out) {
		return nil, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	start = time.Now()
	var hashRoot [32]byte
	for i := 0; i < iterations; i++ {
		root, err := t.Message.HashTreeRoot()
		if err != nil {
			return nil, nil, fmt.Errorf("hashroot error: %v", err)
		}
		hashRoot = root
	}
	hashTime = time.Since(start)

	//out, _ = yaml.Marshal(t)
	//fmt.Printf("%v\n\n", string(out))

	return []time.Duration{unmarshalTime, marshalTime, hashTime}, [][32]byte{hashRoot}, nil
}

func test_state_fastssz(in []byte, iterations int) ([]time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)
	hashTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)
		err := t.UnmarshalSSZ(in)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.BeaconState)
	t.UnmarshalSSZ(in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := t.MarshalSSZ()
		if err != nil {
			return nil, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZ()
	if !bytes.Equal(in, out) {
		return nil, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	start = time.Now()
	var hashRoot [32]byte
	for i := 0; i < iterations; i++ {
		root, err := t.HashTreeRoot()
		if err != nil {
			return nil, nil, fmt.Errorf("hashroot error: %v", err)
		}
		hashRoot = root
	}
	hashTime = time.Since(start)

	return []time.Duration{unmarshalTime, marshalTime, hashTime}, [][32]byte{hashRoot}, nil
}

func test_block_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) ([]time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)
	hashTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.SignedBeaconBlock)
		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.SignedBeaconBlock)
	dynssz.UnmarshalSSZ(t, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := dynssz.MarshalSSZ(t)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := dynssz.MarshalSSZ(t)
	if !bytes.Equal(in, out) {
		return nil, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	start = time.Now()
	var hashRoot [32]byte
	for i := 0; i < iterations; i++ {
		root, err := dynssz.HashTreeRoot(t.Message)
		if err != nil {
			return nil, nil, fmt.Errorf("hashroot error: %v", err)
		}
		hashRoot = root
	}
	hashTime = time.Since(start)

	//out, _ = yaml.Marshal(t)
	//fmt.Printf("%v\n\n", string(out))

	return []time.Duration{unmarshalTime, marshalTime, hashTime}, [][32]byte{hashRoot}, nil
}

func test_state_dynssz(dynssz *ssz.DynSsz, in []byte, iterations int) ([]time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)
	hashTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(deneb.BeaconState)

		err := dynssz.UnmarshalSSZ(t, in)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(deneb.BeaconState)
	dynssz.UnmarshalSSZ(t, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, err := dynssz.MarshalSSZ(t)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := dynssz.MarshalSSZ(t)
	if !bytes.Equal(in, out) {
		return nil, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	start = time.Now()
	var hashRoot [32]byte
	for i := 0; i < iterations; i++ {
		root, err := dynssz.HashTreeRoot(t)
		if err != nil {
			return nil, nil, fmt.Errorf("hashroot error: %v", err)
		}
		hashRoot = root
	}
	hashTime = time.Since(start)

	return []time.Duration{unmarshalTime, marshalTime, hashTime}, [][32]byte{hashRoot}, nil
}

func test_block_dynssz_codegen(dynssz *ssz.DynSsz, in []byte, iterations int) ([]time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)
	hashTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(TestBeaconBlock)
		err := t.UnmarshalSSZDyn(dynssz, in)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(TestBeaconBlock)
	t.UnmarshalSSZDyn(dynssz, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		size := t.SizeSSZDyn(dynssz)
		buf := make([]byte, 0, size)
		_, err := t.MarshalSSZDyn(dynssz, buf)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZDyn(dynssz, nil)
	if !bytes.Equal(in, out) {
		for i := 0; i < len(in); i++ {
			if in[i] != out[i] {
				fmt.Printf("%d:  0x%x != 0x%x\n", i, in[i], out[i])
				break
			}
		}

		return nil, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	start = time.Now()
	var hashRoot [32]byte
	for i := 0; i < iterations; i++ {
		root, err := dynssz.HashTreeRoot(t.Message)
		if err != nil {
			return nil, nil, fmt.Errorf("hashroot error: %v", err)
		}
		hashRoot = root
	}
	hashTime = time.Since(start)

	//out, _ = yaml.Marshal(t)
	//fmt.Printf("%v\n\n", string(out))

	return []time.Duration{unmarshalTime, marshalTime, hashTime}, [][32]byte{hashRoot}, nil
}

func test_state_dynssz_codegen(dynssz *ssz.DynSsz, in []byte, iterations int) ([]time.Duration, [][32]byte, error) {
	unmarshalTime := time.Duration(0)
	marshalTime := time.Duration(0)
	hashTime := time.Duration(0)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := new(TestBeaconState)

		err := t.UnmarshalSSZDyn(dynssz, in)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal error: %v", err)
		}
	}
	unmarshalTime = time.Since(start)

	t := new(TestBeaconState)
	t.UnmarshalSSZDyn(dynssz, in)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		size := t.SizeSSZDyn(dynssz)
		buf := make([]byte, size)
		var err error
		_, err = t.MarshalSSZDyn(dynssz, buf[:0])
		if err != nil {
			return nil, nil, fmt.Errorf("marshal error: %v", err)
		}
	}
	marshalTime = time.Since(start)

	out, _ := t.MarshalSSZDyn(dynssz, nil)
	if !bytes.Equal(in, out) {
		for i := 0; i < len(in); i++ {
			if in[i] != out[i] {
				fmt.Printf("%d:  0x%x != 0x%x\n", i, in[i], out[i])
				break
			}
		}
		return nil, nil, fmt.Errorf("SSZ mismatch after re-marshalling")
	}

	start = time.Now()
	var hashRoot [32]byte
	for i := 0; i < iterations; i++ {
		root, err := dynssz.HashTreeRoot(t)
		if err != nil {
			return nil, nil, fmt.Errorf("hashroot error: %v", err)
		}
		hashRoot = root
	}
	hashTime = time.Since(start)

	return []time.Duration{unmarshalTime, marshalTime, hashTime}, [][32]byte{hashRoot}, nil
}
