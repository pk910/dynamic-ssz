// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/hex"
	"fmt"

	dynssz "github.com/pk910/dynamic-ssz"
)

// go run test_progressive_container.go

// Test case 1: active fields at indices 0, 1, 3, 5, 7
type TestProgressive1 struct {
	A uint64   `ssz-index:"0"`
	B uint64   `ssz-index:"1"`
	D uint16   `ssz-index:"3"`
	F [32]byte `ssz-index:"5"`
	H uint64   `ssz-index:"7"`
}

// Test case 2: active fields at indices 0, 2, 4, 6
type TestProgressive2 struct {
	A uint64   `ssz-index:"0"`
	C [32]byte `ssz-index:"2"`
	E uint64   `ssz-index:"4"`
	G uint16   `ssz-index:"6"`
}

// Test case 3: active fields at indices 0, 1, 2
type TestProgressive3 struct {
	A uint64   `ssz-index:"0"`
	B uint64   `ssz-index:"1"`
	C [32]byte `ssz-index:"2"`
}

func main() {
	ds := dynssz.NewDynSsz(nil)

	fmt.Println("=== Progressive Container Test (Go) ===")

	// Test case 1: active fields at indices 0, 1, 3, 5, 7
	fmt.Println("\nTest Case 1:")
	fmt.Println("Active field indices: 0, 1, 3, 5, 7")

	var fBytes1 [32]byte
	for i := 0; i < 32; i++ {
		fBytes1[i] = 0x11
	}

	test1 := TestProgressive1{
		A: 12345,
		B: 67890,
		D: 999,
		F: fBytes1,
		H: 0xdeadbeef,
	}

	ssz1, err := ds.MarshalSSZ(test1)
	if err != nil {
		fmt.Printf("SSZ encoding error: %v\n", err)
	} else {
		fmt.Printf("SSZ encoded: %x\n", ssz1)
		fmt.Printf("SSZ length: %d bytes\n", len(ssz1))
	}

	root1, err := ds.HashTreeRoot(test1)
	if err != nil {
		fmt.Printf("Hash tree root error: %v\n", err)
	} else {
		fmt.Printf("Tree root: %x\n", root1)
	}

	// Compare with Python results
	expectedSSZ1 := "39300000000000003209010000000000e7031111111111111111111111111111111111111111111111111111111111111111efbeadde00000000"
	expectedRoot1 := "816edda1de1b98f441d929ac0d5a4dfb08c059df22a22fae9add106dadf0364c"

	if hex.EncodeToString(ssz1) == expectedSSZ1 {
		fmt.Println("✓ SSZ matches Python implementation")
	} else {
		fmt.Println("✗ SSZ does not match Python implementation")
		fmt.Printf("  Expected: %s\n", expectedSSZ1)
		fmt.Printf("  Got:      %x\n", ssz1)
	}

	if hex.EncodeToString(root1[:]) == expectedRoot1 {
		fmt.Println("✓ Root matches Python implementation")
	} else {
		fmt.Println("✗ Root does not match Python implementation")
		fmt.Printf("  Expected: %s\n", expectedRoot1)
		fmt.Printf("  Got:      %x\n", root1)
	}

	// Test case 2: active fields at indices 0, 2, 4, 6
	fmt.Println("\nTest Case 2:")
	fmt.Println("Active field indices: 0, 2, 4, 6")

	var cBytes2 [32]byte
	for i := 0; i < 32; i++ {
		cBytes2[i] = 0x22
	}

	test2 := TestProgressive2{
		A: 11111,
		C: cBytes2,
		E: 33333,
		G: 444,
	}

	ssz2, err := ds.MarshalSSZ(test2)
	if err != nil {
		fmt.Printf("SSZ encoding error: %v\n", err)
	} else {
		fmt.Printf("SSZ encoded: %x\n", ssz2)
		fmt.Printf("SSZ length: %d bytes\n", len(ssz2))
	}

	root2, err := ds.HashTreeRoot(test2)
	if err != nil {
		fmt.Printf("Hash tree root error: %v\n", err)
	} else {
		fmt.Printf("Tree root: %x\n", root2)
	}

	// Compare with Python results
	expectedSSZ2 := "672b00000000000022222222222222222222222222222222222222222222222222222222222222223582000000000000bc01"
	expectedRoot2 := "51e5fe5ce5c39cee27fd091d3de9b73d90e43a3d8fcf85dd96baa22db0aa6aef"

	if hex.EncodeToString(ssz2) == expectedSSZ2 {
		fmt.Println("✓ SSZ matches Python implementation")
	} else {
		fmt.Println("✗ SSZ does not match Python implementation")
		fmt.Printf("  Expected: %s\n", expectedSSZ2)
		fmt.Printf("  Got:      %x\n", ssz2)
	}

	if hex.EncodeToString(root2[:]) == expectedRoot2 {
		fmt.Println("✓ Root matches Python implementation")
	} else {
		fmt.Println("✗ Root does not match Python implementation")
		fmt.Printf("  Expected: %s\n", expectedRoot2)
		fmt.Printf("  Got:      %x\n", root2)
	}

	// Test case 3: active fields at indices 0, 1, 2
	fmt.Println("\nTest Case 3:")
	fmt.Println("Active field indices: 0, 1, 2")

	var cBytes3 [32]byte
	for i := 0; i < 32; i++ {
		cBytes3[i] = 0x33
	}

	test3 := TestProgressive3{
		A: 99999,
		B: 88888,
		C: cBytes3,
	}

	ssz3, err := ds.MarshalSSZ(test3)
	if err != nil {
		fmt.Printf("SSZ encoding error: %v\n", err)
	} else {
		fmt.Printf("SSZ encoded: %x\n", ssz3)
		fmt.Printf("SSZ length: %d bytes\n", len(ssz3))
	}

	root3, err := ds.HashTreeRoot(test3)
	if err != nil {
		fmt.Printf("Hash tree root error: %v\n", err)
	} else {
		fmt.Printf("Tree root: %x\n", root3)
	}

	// Compare with Python results
	expectedSSZ3 := "9f86010000000000385b0100000000003333333333333333333333333333333333333333333333333333333333333333"
	expectedRoot3 := "882e46351726c479519a1a51d821f4acbe3cb99531a64eb6ff8c393b7676180c"

	if hex.EncodeToString(ssz3) == expectedSSZ3 {
		fmt.Println("✓ SSZ matches Python implementation")
	} else {
		fmt.Println("✗ SSZ does not match Python implementation")
		fmt.Printf("  Expected: %s\n", expectedSSZ3)
		fmt.Printf("  Got:      %x\n", ssz3)
	}

	if hex.EncodeToString(root3[:]) == expectedRoot3 {
		fmt.Println("✓ Root matches Python implementation")
	} else {
		fmt.Println("✗ Root does not match Python implementation")
		fmt.Printf("  Expected: %s\n", expectedRoot3)
		fmt.Printf("  Got:      %x\n", root3)
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("All test cases compare SSZ encoding and hash tree root")
	fmt.Println("between Go dynamic-ssz and Python remerkleable implementations")
	fmt.Println("for ProgressiveContainer with different active field patterns.")
}
