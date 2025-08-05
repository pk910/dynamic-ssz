#!/usr/bin/env python3

from remerkleable.core import View
from remerkleable.basic import uint16
from remerkleable.complex import Container
from remerkleable.progressive import ProgressiveBitlist


class TestStruct(Container):
    F1: ProgressiveBitlist  # Progressive bitlist


def main():
    # Create test data with 1000 bits: bits 0-999 where every 3rd bit is set
    # This creates a pattern: False, False, True, False, False, True, ...
    bit_values = [(i % 3 == 2) for i in range(1000)]
    
    # Create the struct with progressive bitlist
    test_data = TestStruct(
        F1=ProgressiveBitlist(*bit_values)
    )
    
    print("=== Progressive Bitlist Test ===")
    print(f"F1 length: {len(test_data.F1)} bits")
    print(f"F1 first 20 bits: {[test_data.F1[i] for i in range(20)]}")
    print(f"F1 last 20 bits: {[test_data.F1[i] for i in range(980, 1000)]}")
    
    # Count set bits
    set_bits = sum(1 for bit in test_data.F1 if bit)
    print(f"Number of set bits: {set_bits} (expected: {len([i for i in range(1000) if i % 3 == 2])})")
    
    # Get SSZ encoding
    try:
        ssz_encoded = test_data.encode_bytes()
        print(f"\nSSZ encoded length: {len(ssz_encoded)} bytes")
        print(f"SSZ encoded (hex): {ssz_encoded.hex()}")
        
        # Show first and last few bytes for clarity
        if len(ssz_encoded) > 40:
            print(f"SSZ first 20 bytes: {ssz_encoded[:20].hex()}")
            print(f"SSZ last 20 bytes: {ssz_encoded[-20:].hex()}")
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
    
    # Show the progressive merkleization structure
    print("\n=== Progressive Bitlist Merkleization Structure ===")
    print("Progressive bitlist creates a tree with increasing subtree sizes:")
    print("- Each chunk holds 256 bits (32 bytes)")
    print("- First subtree: 1 chunk (256 bits)")
    print("- Second subtree: 4 chunks (1024 bits)")
    print("- Third subtree: 16 chunks (4096 bits)")
    print("- Fourth subtree: 64 chunks (16384 bits)")
    print(f"Total bits: {len(test_data.F1)}")
    
    # Calculate which subtrees contain our 1000 bits
    remaining_bits = 1000
    subtree_size = 256  # bits per chunk * 1 chunk
    subtree_num = 1
    print("\nBit distribution across subtrees:")
    while remaining_bits > 0:
        bits_in_subtree = min(remaining_bits, subtree_size)
        chunks_used = (bits_in_subtree + 255) // 256  # ceiling division
        print(f"Subtree {subtree_num}: {bits_in_subtree} bits ({chunks_used} chunks, capacity: {subtree_size} bits)")
        remaining_bits -= bits_in_subtree
        subtree_size *= 4
        subtree_num += 1
    
    # Test different sizes of progressive bitlists
    print("\n=== Testing Various Sizes ===")
    test_sizes = [10, 100, 256, 257, 500, 1024, 2000, 5000]
    
    for size in test_sizes:
        # Create a bitlist with alternating pattern
        bits = [(i % 2 == 1) for i in range(size)]
        test_bl = TestStruct(F1=ProgressiveBitlist(*bits))
        
        try:
            ssz = test_bl.encode_bytes()
            root = test_bl.hash_tree_root()
            print(f"Size {size}: SSZ length={len(ssz)} bytes, root={root.hex()[:16]}...")
        except Exception as e:
            print(f"Size {size}: Error - {e}")
    
    # Test edge cases
    print("\n=== Edge Cases ===")
    
    # Empty bitlist
    empty_test = TestStruct(F1=ProgressiveBitlist())
    print(f"Empty bitlist: length={len(empty_test.F1)}, root={empty_test.hash_tree_root().hex()}")
    
    # Single bit
    single_test = TestStruct(F1=ProgressiveBitlist(True))
    print(f"Single bit (True): length={len(single_test.F1)}, root={single_test.hash_tree_root().hex()}")
    
    # All zeros
    zeros_test = TestStruct(F1=ProgressiveBitlist(*[False] * 100))
    print(f"100 zeros: length={len(zeros_test.F1)}, root={zeros_test.hash_tree_root().hex()}")
    
    # All ones
    ones_test = TestStruct(F1=ProgressiveBitlist(*[True] * 100))
    print(f"100 ones: length={len(ones_test.F1)}, root={ones_test.hash_tree_root().hex()}")
    
    # Test direct progressive bitlist (without container)
    print("\n=== Direct Progressive Bitlist (without container) ===")
    direct_bitlist = ProgressiveBitlist(*bit_values)
    direct_root = direct_bitlist.hash_tree_root()
    print(f"Direct progressive bitlist root: {direct_root.hex()}")
    
    # Compare with bitlist field root
    print(f"F1 field root should be different due to container merkleization")
    
    # Debug internal structure
    print("\n=== Debug Internal Structure ===")
    try:
        backing = direct_bitlist.get_backing()
        # ProgressiveBitlist stores data in left child and length in right child
        data_node = backing.get_left()
        length_node = backing.get_right()
        
        print(f"Length value: {direct_bitlist.length()}")
        print(f"Length node hash: {length_node.merkle_root().hex()}")
        print(f"Data node hash: {data_node.merkle_root().hex()}")
        
        # The final root should be hash(data_root, length_root)
        import hashlib
        manual_root = hashlib.sha256(data_node.merkle_root() + length_node.merkle_root()).digest()
        print(f"Manual root calculation: {manual_root.hex()}")
        print(f"Matches direct root: {manual_root.hex() == direct_root.hex()}")
        
    except Exception as e:
        print(f"Error accessing internal structure: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    main()