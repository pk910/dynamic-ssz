#!/usr/bin/env python3

import sys
import os
# Add local remerkleable to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'remerkleable'))

from remerkleable.core import View
from remerkleable.basic import uint16, uint64
from remerkleable.byte_arrays import Bytes32
from remerkleable.complex import Container
from remerkleable.progressive import ProgressiveContainer


# Define a base container with all possible fields
class TestBase(Container):
    a: uint64        # index 0
    b: uint64        # index 1  
    c: Bytes32       # index 2
    d: uint16        # index 3
    e: uint64        # index 4
    f: Bytes32       # index 5
    g: uint16        # index 6
    h: uint64        # index 7


def main():
    # Test case 1: Enable fields at indices 0, 1, 3, 5, 7 (skip 2, 4, 6)
    # Active fields: [1, 1, 0, 1, 0, 1, 0, 1]
    print("=== Progressive Container Test ===")
    
    # Create a progressive container class with specific active fields
    active_fields = [1, 1, 0, 1, 0, 1, 0, 1]
    TestProgressive = ProgressiveContainer(active_fields)
    
    # Define the container structure dynamically
    class TestStruct(TestProgressive):
        a: uint64        # active (index 0)
        b: uint64        # active (index 1)  
        d: uint16        # active (index 3)
        f: Bytes32       # active (index 5)
        h: uint64        # active (index 7)
    
    # Create test data
    test_data = TestStruct(
        a=uint64(12345),
        b=uint64(67890),
        d=uint16(999),
        f=Bytes32(b'\x11' * 32),
        h=uint64(0xdeadbeef)
    )
    
    print(f"Active fields: a={test_data.a}, b={test_data.b}, d={test_data.d}, f={test_data.f.hex()}, h={test_data.h}")
    print(f"Active field pattern: {active_fields}")
    
    # Get SSZ encoding
    try:
        ssz_encoded = test_data.encode_bytes()
        print(f"\nSSZ encoded: {ssz_encoded.hex()}")
        print(f"SSZ length: {len(ssz_encoded)} bytes")
    except Exception as e:
        print(f"\nSSZ encoding error: {e}")
        import traceback
        traceback.print_exc()
    
    # Get hash tree root
    try:
        tree_root = test_data.hash_tree_root()
        print(f"\nTree root: {tree_root.hex()}")
    except Exception as e:
        print(f"\nTree root error: {e}")
        import traceback
        traceback.print_exc()
    
    # Test case 2: Different active fields pattern
    print("\n=== Test Case 2: Different Active Fields ===")
    active_fields2 = [1, 0, 1, 0, 1, 0, 1]  # indices 0, 2, 4, 6
    TestProgressive2 = ProgressiveContainer(active_fields2)
    
    class TestStruct2(TestProgressive2):
        a: uint64        # active (index 0)
        c: Bytes32       # active (index 2)
        e: uint64        # active (index 4)
        g: uint16        # active (index 6)
    
    test_data2 = TestStruct2(
        a=uint64(11111),
        c=Bytes32(b'\x22' * 32),
        e=uint64(33333),
        g=uint16(444)
    )
    
    print(f"Active fields: a={test_data2.a}, c={test_data2.c.hex()}, e={test_data2.e}, g={test_data2.g}")
    print(f"Active field pattern: {active_fields2}")
    
    try:
        ssz_encoded2 = test_data2.encode_bytes()
        print(f"\nSSZ encoded: {ssz_encoded2.hex()}")
        print(f"SSZ length: {len(ssz_encoded2)} bytes")
        
        tree_root2 = test_data2.hash_tree_root()
        print(f"Tree root: {tree_root2.hex()}")
    except Exception as e:
        print(f"\nError in test case 2: {e}")
        import traceback
        traceback.print_exc()
    
    # Test case 3: Minimal case - only first few fields 
    print("\n=== Test Case 3: Only First Few Fields ===")
    active_fields3 = [1, 1, 1]  # Only first 3 fields active
    TestProgressive3 = ProgressiveContainer(active_fields3)
    
    class TestStruct3(TestProgressive3):
        a: uint64        # active (index 0)
        b: uint64        # active (index 1)
        c: Bytes32       # active (index 2)
    
    test_data3 = TestStruct3(
        a=uint64(99999),
        b=uint64(88888),
        c=Bytes32(b'\x33' * 32)
    )
    
    print(f"Active fields: a={test_data3.a}, b={test_data3.b}, c={test_data3.c.hex()}")
    print(f"Active field pattern: {active_fields3}")
    
    try:
        ssz_encoded3 = test_data3.encode_bytes()
        print(f"\nSSZ encoded: {ssz_encoded3.hex()}")
        print(f"SSZ length: {len(ssz_encoded3)} bytes")
        
        tree_root3 = test_data3.hash_tree_root()
        print(f"Tree root: {tree_root3.hex()}")
    except Exception as e:
        print(f"\nError in test case 3: {e}")
        import traceback
        traceback.print_exc()
    
    # Output results in a format we can use for Go comparison
    print("\n=== Results Summary for Go Implementation ===")
    print("Test Case 1:")
    print(f"  Active: [1, 1, 0, 1, 0, 1, 0, 1]")
    print(f"  SSZ: {ssz_encoded.hex() if 'ssz_encoded' in locals() else 'ERROR'}")
    print(f"  Root: {tree_root.hex() if 'tree_root' in locals() else 'ERROR'}")
    print("\nTest Case 2:")
    print(f"  Active: [1, 0, 1, 0, 1, 0, 1]")
    print(f"  SSZ: {ssz_encoded2.hex() if 'ssz_encoded2' in locals() else 'ERROR'}")
    print(f"  Root: {tree_root2.hex() if 'tree_root2' in locals() else 'ERROR'}")
    print("\nTest Case 3:")
    print(f"  Active: [1, 1, 1]")
    print(f"  SSZ: {ssz_encoded3.hex() if 'ssz_encoded3' in locals() else 'ERROR'}")
    print(f"  Root: {tree_root3.hex() if 'tree_root3' in locals() else 'ERROR'}")


if __name__ == "__main__":
    main()