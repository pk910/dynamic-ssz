// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/examples/codegen/types"
	"github.com/pk910/dynamic-ssz/sszutils"
)

func main() {
	fmt.Println("Dynamic SSZ Code Generation Example")
	fmt.Println("===================================")
	fmt.Println()
	fmt.Println("This example demonstrates code generation for high-performance SSZ operations.")
	fmt.Println()
	fmt.Println("Steps to use:")
	fmt.Println("1. Run: go run codegen/codegen.go  (generates optimized code)")
	fmt.Println("2. Run: go run .                   (uses generated methods)")
	fmt.Println()

	// Create a DynSsz instance for reflection-based operations
	ds := dynssz.NewDynSsz(nil)

	// Create sample data
	user := createSampleUser()
	tx := createSampleTransaction()
	block := createSampleBlock()
	game := createSampleGame()

	fmt.Println("Testing with reflection-based dynamic SSZ:")
	fmt.Println()

	fmt.Println("1. User struct:")
	testWithReflection("User", user, ds)

	fmt.Println("\n2. Transaction struct:")
	testWithReflection("Transaction", tx, ds)

	fmt.Println("\n3. Block struct:")
	testWithReflection("Block", block, ds)

	fmt.Println("\n4. GameState struct:")
	testWithReflection("GameState", game, ds)

	useri := interface{}(user)
	_, hasCode := useri.(sszutils.DynamicMarshaler)
	testType := "reflection"
	if hasCode {
		testType = "generated code"
	}
	fmt.Printf("\n5. Performance test (%s):\n", testType)
	performanceTest(user, ds)

	fmt.Println()
	fmt.Println("To see 2-3x performance improvement:")
	fmt.Println("1. Run 'go run codegen/codegen.go' to generate optimized code")
	fmt.Println("2. Run 'go run .' again to use generated methods")
}

func testWithReflection(name string, obj interface{}, ds *dynssz.DynSsz) {
	// Marshal
	data, err := ds.MarshalSSZ(obj)
	if err != nil {
		log.Printf("Marshal error: %v", err)
		return
	}
	fmt.Printf("  %s marshaled to %d bytes", name, len(data))

	// Hash tree root
	root, err := ds.HashTreeRoot(obj)
	if err != nil {
		log.Printf("HashTreeRoot error: %v", err)
		return
	}
	fmt.Printf(" | hash: 0x%x", root)
}

func performanceTest(user *types.User, ds *dynssz.DynSsz) {
	const iterations = 10000

	// Warm up
	for i := 0; i < 100; i++ {
		ds.MarshalSSZ(user)
		ds.HashTreeRoot(user)
	}

	// Marshal benchmark
	start := time.Now()
	for i := 0; i < iterations; i++ {
		ds.MarshalSSZ(user)
	}
	marshalTime := time.Since(start)

	// Hash benchmark
	userHasher, hasHasher := (interface{}(user)).(sszutils.FastsszHashRoot)
	start = time.Now()
	for i := 0; i < iterations; i++ {
		if hasHasher {
			userHasher.HashTreeRoot()
		} else {
			ds.HashTreeRoot(user)
		}
	}
	hashTime := time.Since(start)

	fmt.Printf(" Performance (%d iterations):\n", iterations)
	fmt.Printf("  - Marshal:   %v (%.0f ns/op)\n", marshalTime, float64(marshalTime.Nanoseconds())/float64(iterations))
	fmt.Printf("  - HashRoot:  %v (%.0f ns/op)\n", hashTime, float64(hashTime.Nanoseconds())/float64(iterations))
}

func createSampleUser() *types.User {
	var pubkey [48]byte
	copy(pubkey[:], "sample-public-key-data-here-for-testing-purposes")

	var balance [32]byte
	copy(balance[:], "\x01\x00\x00\x00\x00\x00\x00\x00") // 1 ETH in wei

	return &types.User{
		ID:      12345,
		Name:    []byte("John Doe"),
		Email:   []byte("john@example.com"),
		PubKey:  pubkey,
		Balance: balance,
		Active:  true,
		Roles:   []byte{0xFF, 0x00, 0x01}, // Some role bits
	}
}

func createSampleTransaction() *types.Transaction {
	var from, to [20]byte
	copy(from[:], "\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0A\x0B\x0C\x0D\x0E\x0F\x10\x11\x12\x13\x14")
	copy(to[:], "\x14\x13\x12\x11\x10\x0F\x0E\x0D\x0C\x0B\x0A\x09\x08\x07\x06\x05\x04\x03\x02\x01")

	var value, gasPrice [32]byte
	copy(value[:], "\x01\x00\x00\x00\x00\x00\x00\x00") // 1 ETH
	copy(gasPrice[:], "\x04\xA8\x17\xC8\x00")          // 20 Gwei

	var signature [65]byte
	copy(signature[:], "sample-signature-data-for-transaction-testing-here")

	return &types.Transaction{
		From:      from,
		To:        to,
		Value:     value,
		Data:      []byte("Hello, SSZ!"),
		Nonce:     42,
		GasLimit:  21000,
		GasPrice:  gasPrice,
		Signature: signature,
	}
}

func createSampleBlock() *types.Block {
	var parentHash, stateRoot, txRoot [32]byte
	copy(parentHash[:], "sample-parent-hash-data-here")
	copy(stateRoot[:], "sample-state-root-data-here")
	copy(txRoot[:], "sample-tx-root-data-here")

	var difficulty [32]byte
	copy(difficulty[:], "\x01\x00\x00\x00\x00\x00\x00\x00") // Some difficulty

	var miner [20]byte
	copy(miner[:], "\xAA\xBB\xCC\xDD\xEE\xFF\x11\x22\x33\x44\x55\x66\x77\x88\x99\x00\x11\x22\x33\x44")

	// Create some sample transactions
	tx1 := createSampleTransaction()
	tx2 := createSampleTransaction()
	tx2.Nonce = 43

	return &types.Block{
		Number:       1000000,
		Timestamp:    uint64(1234567890),
		ParentHash:   parentHash,
		StateRoot:    stateRoot,
		TxRoot:       txRoot,
		Difficulty:   difficulty,
		Transactions: []types.Transaction{*tx1, *tx2},
		Miner:        miner,
		ExtraData:    []byte("Mined with Dynamic SSZ"),
	}
}

func createSampleGame() *types.GameState {
	// Create a sample chess board position
	var board [8][8]uint8
	// Set up some pieces (simplified)
	board[0][0] = 1  // Rook
	board[0][1] = 2  // Knight
	board[0][4] = 6  // King
	board[7][4] = 12 // Enemy king

	// Sample players
	var player1ID, player2ID []byte
	copy(player1ID[:], "player-1-uuid-16")
	copy(player2ID[:], "player-2-uuid-16")

	players := []types.Player{
		{
			ID:    player1ID,
			Name:  []byte("Alice"),
			Score: 1200,
			Team:  1,
		},
		{
			ID:    player2ID,
			Name:  []byte("Bob"),
			Score: 1150,
			Team:  2,
		},
	}

	// Sample moves
	moves := []types.Move{
		{Player: 1, FromX: 4, FromY: 1, ToX: 4, ToY: 3, Timestamp: uint64(1234567890)},
		{Player: 2, FromX: 4, FromY: 6, ToX: 4, ToY: 4, Timestamp: uint64(1234567890)},
	}

	// Sample map tiles (10x16 grid)
	mapTiles := make([][]types.Tile, 10)
	for i := range mapTiles {
		mapTiles[i] = make([]types.Tile, 16)
		for j := range mapTiles[i] {
			mapTiles[i][j] = types.Tile{
				Type:    uint8((i + j) % 4), // Vary tile types
				Height:  uint8(i % 8),       // Vary heights
				Owner:   uint8((i * j) % 3), // Vary owners
				HasFlag: (i+j)%5 == 0,       // Some tiles have flags
			}
		}
	}

	return &types.GameState{
		Round:      42,
		IsActive:   true,
		Board:      board,
		Players:    players,
		Moves:      moves,
		MapTiles:   mapTiles,
		ActiveMask: []byte{0xFF, 0x03}, // First 10 players active
	}
}
