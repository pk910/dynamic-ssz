#!/usr/bin/env python3

from ssz import (
    Serializable,
    uint16,
    List,
    Vector,
    encode,
    get_hash_tree_root,
)
from dataclasses import dataclass
from typing import Type


class TestData(Serializable):
    fields = (
        ('T1', uint16),
        ('T2', uint16),
    )


class TestDataYSS(Serializable):
    fields = (
        ('Foo', List(List(TestData, 100), 100)),
    )


class TestDataXS(Serializable):
    fields = (
        ('XS', List(TestData, 100)),
    )


class TestDataXSS(Serializable):
    fields = (
        ('Foo', List(TestDataXS, 100)),
    )


class TestData3D(Serializable):
    fields = (
        ('Data', List(List(List(TestData, 100), 100), 100)),
    )


class TestData3DWithVector(Serializable):
    fields = (
        ('Data', List(Vector(List(TestData, 100), 2), 100)),
    )


class TestData4DWithVector(Serializable):
    fields = (
        ('Data', List(Vector(List(List(TestData, 100), 100), 2), 100)),
    )


def main():
    # Create the test data matching the Go example
    test_data1 = TestData(
        T1=1,
        T2=2,
    )
    test_data2 = TestData(
        T1=1003,
        T2=1004,
    )
    test_data3 = TestData(
        T1=5,
        T2=6,
    )
    test_data4 = TestData(
        T1=1007,
        T2=1008,
    )
    
    print("\n=== TestDataXSS (nested structure) ===")
    
    dx = TestDataXSS(
        Foo=[TestDataXS(XS=[test_data1, test_data2]), TestDataXS(XS=[test_data3, test_data4])]
    )
    
    # Marshal SSZ
    try:
        dx_ssz = encode(dx)
        print(f"dx ssz: {dx_ssz.hex()}")
    except Exception as e:
        print(f"dx ssz err: {e}")
    
    # Hash tree root
    try:
        dx_root = get_hash_tree_root(dx)
        print(f"dx root: {dx_root.hex()}")
    except Exception as e:
        print(f"dx root err: {e}")

    print("\n=== TestDataYSS (multi-dimensional slice) ===")

    dy = TestDataYSS(
        Foo=[[test_data1, test_data2], [test_data3, test_data4]]
    )

    # Marshal SSZ
    try:
        dy_ssz = encode(dy)
        print(f"dy ssz: {dy_ssz.hex()}")
    except Exception as e:
        print(f"dy ssz err: {e}")
    
    # Hash tree root
    try:
        dy_root = get_hash_tree_root(dy)
        print(f"dy root: {dy_root.hex()}")
    except Exception as e:
        print(f"dy root err: {e}")

    print("\n=== TestData3D (3-dimensional slice) ===")
    
    # Create 3D test data
    d3d = TestData3D(
        Data=[
            [
                [test_data1, test_data2],
                [test_data3, test_data4]
            ],
            [
                [test_data2, test_data3],
                [test_data4, test_data1]
            ]
        ]
    )
    
    # Marshal SSZ
    try:
        d3d_ssz = encode(d3d)
        print(f"d3d ssz: {d3d_ssz.hex()}")
    except Exception as e:
        print(f"d3d ssz err: {e}")
    
    # Hash tree root
    try:
        d3d_root = get_hash_tree_root(d3d)
        print(f"d3d root: {d3d_root.hex()}")
    except Exception as e:
        print(f"d3d root err: {e}")

    print("\n=== TestData3DWithVector (3-dimensional with fixed size vector) ===")
    
    # Create 3D test data with vector (fixed size on second dimension)
    d3dv = TestData3DWithVector(
        Data=[
            [  # First element in outermost list
                [  # First element in vector (fixed size 2)
                    test_data1, test_data2, test_data3
                ],
                [  # Second element in vector (fixed size 2)
                    test_data3, test_data4
                ]
            ],
            [  # Second element in outermost list
                [  # First element in vector (fixed size 2)
                    test_data2, test_data3
                ],
                [  # Second element in vector (fixed size 2)
                    test_data4, test_data1
                ]
            ]
        ]
    )
    
    # Marshal SSZ
    try:
        d3dv_ssz = encode(d3dv)
        print(f"d3dv ssz: {d3dv_ssz.hex()}")
    except Exception as e:
        print(f"d3dv ssz err: {e}")
    
    # Hash tree root
    try:
        d3dv_root = get_hash_tree_root(d3dv)
        print(f"d3dv root: {d3dv_root.hex()}")
    except Exception as e:
        print(f"d3dv root err: {e}")

    print("\n=== TestData4DWithVector (4-dimensional with fixed size vector) ===")
    
    # Create 4D test data with vector (fixed size on second dimension)
    d4d = TestData4DWithVector(
        Data=[
            [  # First element in outermost list
                [  # First element in vector (fixed size 2)
                    [test_data1, test_data2],
                    [test_data3, test_data4]
                ],
                [  # Second element in vector (fixed size 2)
                    [test_data2, test_data3],
                    [test_data4, test_data1]
                ]
            ],
            [  # Second element in outermost list
                [  # First element in vector (fixed size 2)
                    [test_data3, test_data4],
                    [test_data1, test_data2]
                ],
                [  # Second element in vector (fixed size 2)
                    [test_data4, test_data1],
                    [test_data2, test_data3]
                ]
            ]
        ]
    )
    
    # Marshal SSZ
    try:
        d4d_ssz = encode(d4d)
        print(f"d4d ssz: {d4d_ssz.hex()}")
    except Exception as e:
        print(f"d4d ssz err: {e}")
    
    # Hash tree root
    try:
        d4d_root = get_hash_tree_root(d4d)
        print(f"d4d root: {d4d_root.hex()}")
    except Exception as e:
        print(f"d4d root err: {e}")


if __name__ == "__main__":
    main()