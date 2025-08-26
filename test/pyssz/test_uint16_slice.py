#!/usr/bin/env python3

from ssz import (
    Serializable,
    uint8,
    uint16,
    uint32,
    boolean,
    List,
    Vector,
    encode,
    get_hash_tree_root,
)


class TestStruct(Serializable):
    fields = (
        ('F1', boolean),
        ('F2', List(uint8, 10)),  # dynamic field with max 10
        ('F3', Vector(uint16, 5)),  # static field with size 5
        ('F4', uint32),
    )


def main():
    # Create the test struct matching the Go example:
    # {true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2}, 3}
    test_data = TestStruct(
        F1=True,
        F2=[1, 1, 1, 1],
        F3=[2, 2, 2, 2, 0],  # Need to pad to size 5 with 0
        F4=3
    )
    
    print("=== TestStruct ===")
    print(f"F1 (bool): {test_data.F1}")
    print(f"F2 ([]uint8 max:10): {list(test_data.F2)}")
    print(f"F3 ([]uint16 size:5): {list(test_data.F3)}")
    print(f"F4 (uint32): {test_data.F4}")
    
    # Marshal SSZ
    try:
        ssz_encoded = encode(test_data)
        print(f"\nSSZ encoded: {ssz_encoded.hex()}")
        print(f"SSZ bytes: {' '.join(f'{b:02x}' for b in ssz_encoded)}")
        print(f"SSZ length: {len(ssz_encoded)} bytes")
    except Exception as e:
        print(f"\nSSZ encoding error: {e}")
    
    # Hash tree root
    try:
        tree_root = get_hash_tree_root(test_data)
        print(f"\nTree root: {tree_root.hex()}")
    except Exception as e:
        print(f"\nTree root error: {e}")
    
    # Show the SSZ structure breakdown
    print("\n=== SSZ Structure Breakdown ===")
    if 'ssz_encoded' in locals():
        offset = 0
        # F1 (bool) - 1 byte
        print(f"F1 (bool): {ssz_encoded[offset:offset+1].hex()} at offset {offset}")
        offset += 1
        
        # F2 offset (dynamic field) - 4 bytes
        f2_offset = int.from_bytes(ssz_encoded[offset:offset+4], 'little')
        print(f"F2 offset: {ssz_encoded[offset:offset+4].hex()} = {f2_offset} at offset {offset}")
        offset += 4
        
        # F3 ([]uint16 size:5) - 10 bytes (5 * 2)
        print(f"F3 ([]uint16): {ssz_encoded[offset:offset+10].hex()} at offset {offset}")
        offset += 10
        
        # F4 (uint32) - 4 bytes
        print(f"F4 (uint32): {ssz_encoded[offset:offset+4].hex()} at offset {offset}")
        offset += 4
        
        # F2 data (dynamic)
        print(f"F2 data: {ssz_encoded[f2_offset:].hex()} at offset {f2_offset}")


if __name__ == "__main__":
    main()