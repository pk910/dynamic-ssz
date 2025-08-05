#!/usr/bin/env python3

from remerkleable.core import View
from remerkleable.basic import uint16
from remerkleable.complex import Container
from remerkleable.progressive import ProgressiveList


class TestStruct(Container):
    F1: ProgressiveList[uint16]  # Progressive list of uint16


def main():
    # Create the test data matching the Go example
    values = list(range(1, 101))  # [1, 2, 3, ..., 100]
    
    # Create the struct with progressive list
    test_data = TestStruct(
        F1=ProgressiveList[uint16](*[uint16(v) for v in values])
    )
    
    print("=== Progressive List Test ===")
    print(f"F1 length: {len(test_data.F1)}")
    print(f"F1 first 10 values: {[test_data.F1[i] for i in range(10)]}")
    print(f"F1 last 10 values: {[test_data.F1[i] for i in range(90, 100)]}")
    
    # Get SSZ encoding
    try:
        ssz_encoded = test_data.encode_bytes()
        print(f"\nSSZ encoded: {ssz_encoded.hex()}")
        print(f"SSZ length: {len(ssz_encoded)} bytes")
        
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
    print("\n=== Progressive Merkleization Structure ===")
    print("Progressive list creates a tree with increasing subtree sizes:")
    print("- First subtree: 1 element")
    print("- Second subtree: 4 elements") 
    print("- Third subtree: 16 elements")
    print("- Fourth subtree: 64 elements")
    print("- Fifth subtree: 256 elements (but we only have 100 - 85 = 15 elements here)")
    print(f"Total elements: {len(test_data.F1)}")
    
    # Calculate which subtrees contain our 100 elements
    remaining = 100
    subtree_size = 1
    subtree_num = 1
    print("\nElement distribution across subtrees:")
    while remaining > 0:
        elements_in_subtree = min(remaining, subtree_size)
        print(f"Subtree {subtree_num}: {elements_in_subtree} elements (capacity: {subtree_size})")
        remaining -= elements_in_subtree
        subtree_size *= 4
        subtree_num += 1
    
    # Debug: Show subtree hashes
    print("\n=== Subtree Hashes (Debug) ===")
    try:
        # Access the internal backing node structure
        backing = test_data.F1.get_backing()
        
        # The ProgressiveList structure stores data in left child and length in right child
        data_node = backing.get_left()
        length_node = backing.get_right()
        
        print(f"Length node hash: {length_node.merkle_root().hex()}")
        print(f"Data node hash: {data_node.merkle_root().hex()}")
        
        # Test standalone progressive list without container
        print("\n=== Direct Progressive List (without container) ===")
        direct_list = ProgressiveList[uint16, 1000000](*[uint16(v) for v in values])
        direct_root = direct_list.hash_tree_root()
        print(f"Direct progressive list root: {direct_root.hex()}")
        
        # Try to access subtree nodes
        from remerkleable.tree import NavigationError
        def print_subtree_structure(node, depth=0, prefix=""):
            indent = "  " * depth
            try:
                print(f"{indent}{prefix}Node hash: {node.merkle_root().hex()}")
                if depth < 4:  # Increase depth to see more
                    try:
                        left = node.get_left()
                        print_subtree_structure(left, depth + 1, "Left ")
                    except (NavigationError, AttributeError):
                        pass  # It's a leaf node
                    try:
                        right = node.get_right()
                        print_subtree_structure(right, depth + 1, "Right ")
                    except (NavigationError, AttributeError):
                        pass  # It's a leaf node
            except Exception as e:
                print(f"{indent}{prefix}Error: {e}")
        
        print("\nData subtree structure (first 4 levels):")
        print_subtree_structure(data_node)
        
        # Create a custom progressive list to understand the final steps
        print("\n=== Manual Progressive List Construction ===")
        from remerkleable.tree import PairNode, zero_node
        import hashlib
        
        # Manually build progressive tree to see each step
        def sha256_hash(data):
            return hashlib.sha256(data).digest()
        
        # Get the raw data chunks (100 uint16 values = 200 bytes = 7 chunks when padded)
        raw_data = b''.join(v.to_bytes(2, 'little') for v in values)
        print(f"Raw data length: {len(raw_data)} bytes")
        
        # Pad to 32-byte chunks
        chunks = []
        for i in range(0, len(raw_data), 32):
            chunk = raw_data[i:i+32]
            if len(chunk) < 32:
                chunk = chunk + b'\x00' * (32 - len(chunk))
            chunks.append(chunk)
        print(f"Number of chunks: {len(chunks)}")
        print(f"First chunk: {chunks[0].hex()}")
        print(f"Last chunk: {chunks[-1].hex()}")
        
        # Manual progressive merkleization to match our algorithm
        def manual_progressive_merkle(chunks_list, depth=0):
            if len(chunks_list) == 0:
                return b'\x00' * 32
            
            base_size = 1 << depth
            print(f"  Depth {depth}: base_size={base_size}, chunks={len(chunks_list)}")
            
            if len(chunks_list) <= base_size:
                # Binary merkleization
                padded = chunks_list + [b'\x00' * 32] * (base_size - len(chunks_list))
                layer = padded[:base_size]
                while len(layer) > 1:
                    next_layer = []
                    for i in range(0, len(layer), 2):
                        if i + 1 < len(layer):
                            combined = layer[i] + layer[i + 1]
                        else:
                            combined = layer[i] + b'\x00' * 32
                        next_layer.append(sha256_hash(combined))
                    layer = next_layer
                result = layer[0]
                print(f"    Binary result: {result.hex()}")
                return result
            
            # Split: first base_size chunks go to right (binary), rest go to left (progressive)
            right_chunks = chunks_list[:base_size]
            left_chunks = chunks_list[base_size:]
            
            right_root = manual_progressive_merkle(right_chunks, depth)
            left_root = manual_progressive_merkle(left_chunks, depth + 2)
            
            # Combine: hash(left, right)
            combined = left_root + right_root
            result = sha256_hash(combined)
            print(f"    Combined at depth {depth}: left={left_root.hex()}, right={right_root.hex()}, result={result.hex()}")
            return result
        
        manual_result = manual_progressive_merkle(chunks)
        print(f"\nManual progressive result: {manual_result.hex()}")
        
        # Now check if this is the final result or if there's a length mixin
        print(f"Expected progressive data: {data_node.merkle_root().hex()}")
        
        # Check if there's a length mixin step
        if manual_result.hex() != data_node.merkle_root().hex():
            print("\n=== Length Mixin Investigation ===")
            length_bytes = len(values).to_bytes(32, 'little')
            print(f"Length ({len(values)}) as 32 bytes: {length_bytes.hex()}")
            
            # Try hash(progressive_root, length)
            with_length = sha256_hash(manual_result + length_bytes)
            print(f"hash(progressive_root, length): {with_length.hex()}")
            
            # Try hash(length, progressive_root)  
            with_length_rev = sha256_hash(length_bytes + manual_result)
            print(f"hash(length, progressive_root): {with_length_rev.hex()}")
            
            if with_length.hex() == data_node.merkle_root().hex():
                print("✓ Found it! Progressive list uses hash(progressive_root, length)")
            elif with_length_rev.hex() == data_node.merkle_root().hex():
                print("✓ Found it! Progressive list uses hash(length, progressive_root)")
        else:
            print("✓ No length mixin needed - direct progressive result matches!")
        
    except Exception as e:
        print(f"Error accessing subtree hashes: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    main()