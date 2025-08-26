package main

import (
	"fmt"

	dynssz "github.com/pk910/dynamic-ssz"
)

// go run test_multidim_slice.go

type TestData struct {
	T1 uint16
	T2 uint16
}

type TestDataXS struct {
	XS []TestData `ssz-max:"100"`
}
type TestDataXSS struct {
	Foo []TestDataXS `ssz-max:"100"`
}

type TestDataYSS struct {
	Foo [][]TestData `ssz-max:"100,100"`
}

type TestData3D struct {
	Data [][][]TestData `ssz-max:"100,100,100"`
}

type TestData3DWithVector struct {
	Data [][2][]TestData `ssz-size:"?,2,?" ssz-max:"100,?,100"`
}

type TestData4DWithVector struct {
	Data [][2][][]TestData `ssz-max:"100,?,100,100"`
}

func main() {
	ds := dynssz.NewDynSsz(nil)

	/*
		fmt.Printf("\n=== TestDataXSS (nested structure) ===\n")
		dx := TestDataXSS{
			Foo: []TestDataXS{
				{
					XS: []TestData{
						{
							T1: 1,
							T2: 2,
						},
						{
							T1: 1003,
							T2: 1004,
						},
					},
				},
				{
					XS: []TestData{
						{
							T1: 5,
							T2: 6,
						},
						{
							T1: 1007,
							T2: 1008,
						},
					},
				},
			},
		}

		dxSsz, err := ds.MarshalSSZ(dx)
		if err != nil {
			fmt.Printf("dx ssz err: %v\n", err)
		} else {
			fmt.Printf("dx ssz: %x\n", dxSsz)
		}

		dxRoot, err := ds.HashTreeRoot(dx)
		if err != nil {
			fmt.Printf("dx root err: %v\n", err)
		} else {
			fmt.Printf("dx root: %x\n", dxRoot)
		}

		err = ds.UnmarshalSSZ(&dx, dxSsz)
		if err != nil {
			fmt.Printf("dx decode err: %v\n", err)
		} else {
			dxJson, _ := json.Marshal(dx)
			fmt.Printf("dx json: %v\n", string(dxJson))
		}

		fmt.Printf("\n=== TestDataYSS (multi-dimensional slice) ===\n")
		dy := TestDataYSS{
			Foo: [][]TestData{
				{
					{
						T1: 1,
						T2: 2,
					},
					{
						T1: 1003,
						T2: 1004,
					},
				},
				{
					{
						T1: 5,
						T2: 6,
					},
					{
						T1: 1007,
						T2: 1008,
					},
				},
			},
		}

		dySsz, err := ds.MarshalSSZ(dy)
		if err != nil {
			fmt.Printf("dy ssz err: %v\n", err)
		} else {
			fmt.Printf("dy ssz: %x\n", dySsz)
		}

		dyRoot, err := ds.HashTreeRoot(dy)
		if err != nil {
			fmt.Printf("dy root err: %v\n", err)
		} else {
			fmt.Printf("dy root: %x\n", dyRoot)
		}

		err = ds.UnmarshalSSZ(&dy, dySsz)
		if err != nil {
			fmt.Printf("dy decode err: %v\n", err)
		} else {
			dyJson, _ := json.Marshal(dy)
			fmt.Printf("dy json: %v\n", string(dyJson))
		}

		fmt.Printf("\n=== TestData3D (3-dimensional slice) ===\n")
		d3d := TestData3D{
			Data: [][][]TestData{
				{
					{
						{T1: 1, T2: 2},
						{T1: 1003, T2: 1004},
					},
					{
						{T1: 5, T2: 6},
						{T1: 1007, T2: 1008},
					},
				},
				{
					{
						{T1: 1003, T2: 1004},
						{T1: 5, T2: 6},
					},
					{
						{T1: 1007, T2: 1008},
						{T1: 1, T2: 2},
					},
				},
			},
		}

		d3dSsz, err := ds.MarshalSSZ(d3d)
		if err != nil {
			fmt.Printf("d3d ssz err: %v\n", err)
		} else {
			fmt.Printf("d3d ssz: %x\n", d3dSsz)
		}

		d3dRoot, err := ds.HashTreeRoot(d3d)
		if err != nil {
			fmt.Printf("d3d root err: %v\n", err)
		} else {
			fmt.Printf("d3d root: %x\n", d3dRoot)
		}
	*/

	fmt.Printf("\n=== TestData3DWithVector (3-dimensional with fixed size vector) ===\n")
	// 3D test case with fixed vector on second dimension
	d3dv := TestData3DWithVector{
		Data: [][2][]TestData{
			{
				// First element in vector (fixed size 2)
				{
					{T1: 1, T2: 2},
					{T1: 1003, T2: 1004},
					{T1: 5, T2: 6},
				},
				// Second element in vector (fixed size 2)
				{
					{T1: 5, T2: 6},
					{T1: 1007, T2: 1008},
				},
			},
			{
				// First element in vector (fixed size 2)
				{
					{T1: 1003, T2: 1004},
					{T1: 5, T2: 6},
				},
				// Second element in vector (fixed size 2)
				{
					{T1: 1007, T2: 1008},
					{T1: 1, T2: 2},
				},
			},
		},
	}

	d3dvSsz, err := ds.MarshalSSZ(d3dv)
	if err != nil {
		fmt.Printf("d3dv ssz err: %v\n", err)
	} else {
		fmt.Printf("d3dv ssz: %x\n", d3dvSsz)
	}

	d3dvRoot, err := ds.HashTreeRoot(d3dv)
	if err != nil {
		fmt.Printf("d3dv root err: %v\n", err)
	} else {
		fmt.Printf("d3dv root: %x\n", d3dvRoot)
	}

	fmt.Printf("\n=== TestData4DWithVector (4-dimensional with fixed size vector) ===\n")
	d4d := TestData4DWithVector{
		Data: [][2][][]TestData{
			{
				// First element in vector (fixed size 2)
				{
					{
						{T1: 1, T2: 2},
						{T1: 1003, T2: 1004},
					},
					{
						{T1: 5, T2: 6},
						{T1: 1007, T2: 1008},
					},
				},
				// Second element in vector (fixed size 2)
				{
					{
						{T1: 1003, T2: 1004},
						{T1: 5, T2: 6},
					},
					{
						{T1: 1007, T2: 1008},
						{T1: 1, T2: 2},
					},
				},
			},
			{
				// First element in vector (fixed size 2)
				{
					{
						{T1: 5, T2: 6},
						{T1: 1007, T2: 1008},
					},
					{
						{T1: 1, T2: 2},
						{T1: 1003, T2: 1004},
					},
				},
				// Second element in vector (fixed size 2)
				{
					{
						{T1: 1007, T2: 1008},
						{T1: 1, T2: 2},
					},
					{
						{T1: 1003, T2: 1004},
						{T1: 5, T2: 6},
					},
				},
			},
		},
	}

	d4dSsz, err := ds.MarshalSSZ(d4d)
	if err != nil {
		fmt.Printf("d4d ssz err: %v\n", err)
	} else {
		fmt.Printf("d4d ssz: %x\n", d4dSsz)
	}

	d4dRoot, err := ds.HashTreeRoot(d4d)
	if err != nil {
		fmt.Printf("d4d root err: %v\n", err)
	} else {
		fmt.Printf("d4d root: %x\n", d4dRoot)
	}
}
