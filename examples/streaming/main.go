// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package main demonstrates streaming SSZ encoding and decoding.
//
// Streaming lets you process large SSZ objects without holding the entire
// serialized form in memory next to the decoded struct. The canonical case
// is downloading a mainnet beacon state (~310 MB serialized) and decoding it
// straight off the HTTP response body with UnmarshalSSZReader — halving peak
// memory versus read-then-unmarshal.
//
// Shown here:
//  1. Stream-encoding to a file with MarshalSSZWriter.
//  2. Stream-decoding from a file with UnmarshalSSZReader and a known size.
//  3. A size-known / size-unknown fallback helper for HTTP bodies
//     (Content-Length present vs chunked responses).
//  4. Network-style streaming through an io.Pipe without any intermediate
//     buffer.
//  5. An allocation comparison between buffered and streamed decoding.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	dynssz "github.com/pk910/dynamic-ssz"
)

// Validator mirrors the consensus-spec validator container (shortened).
type Validator struct {
	Pubkey                [48]byte
	WithdrawalCredentials [32]byte
	EffectiveBalance      uint64
	Slashed               bool
	ActivationEpoch       uint64
	ExitEpoch             uint64
}

// ValidatorRegistry is a large, list-heavy structure — the shape where
// streaming pays off. With 100k validators it serializes to ~11 MB.
type ValidatorRegistry struct {
	Slot       uint64
	BlockRoots [][32]byte  `ssz-size:"64,32"`
	Validators []Validator `ssz-max:"1099511627776"`
	Balances   []uint64    `ssz-max:"1099511627776"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fmt.Println("Streaming Example — SSZ without the intermediate buffer")
	fmt.Println("========================================================")

	// The stream reader buffer defaults to 2 KB. For multi-megabyte payloads
	// a larger buffer avoids many small reads — 1 MB is a good fit for
	// beacon-state-sized downloads.
	ds := dynssz.NewDynSsz(nil, dynssz.WithStreamReaderBufferSize(1<<20))

	registry := buildRegistry(100_000)

	size, err := ds.SizeSSZ(registry)
	if err != nil {
		return fmt.Errorf("failed to compute size: %w", err)
	}

	fmt.Printf("\nRegistry with %d validators serializes to %.1f MB\n",
		len(registry.Validators), float64(size)/(1<<20))

	// 1. Stream-encode directly to a file — no full serialization buffer.
	fmt.Println("\n1. MarshalSSZWriter — stream-encode to a file:")

	file, err := os.CreateTemp("", "registry-*.ssz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(file.Name())

	if err = ds.MarshalSSZWriter(registry, file); err != nil {
		return fmt.Errorf("failed to stream-encode: %w", err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	info, err := os.Stat(file.Name())
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	fmt.Printf("  wrote %d bytes to %s\n", info.Size(), file.Name())

	// 2. Stream-decode from the file. The total size must be known so
	// offsets can be interpreted; for files it comes from Stat.
	fmt.Println("\n2. UnmarshalSSZReader — stream-decode with a known size:")

	reader, err := os.Open(file.Name())
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	var decoded ValidatorRegistry

	err = ds.UnmarshalSSZReader(&decoded, reader, int(info.Size()))

	_ = reader.Close()

	if err != nil {
		return fmt.Errorf("failed to stream-decode: %w", err)
	}

	fmt.Printf("  decoded %d validators, %d balances\n", len(decoded.Validators), len(decoded.Balances))

	originalRoot, err := ds.HashTreeRoot(registry)
	if err != nil {
		return fmt.Errorf("failed to hash original: %w", err)
	}

	decodedRoot, err := ds.HashTreeRoot(&decoded)
	if err != nil {
		return fmt.Errorf("failed to hash decoded: %w", err)
	}

	fmt.Printf("  hash tree roots match: %v\n", originalRoot == decodedRoot)

	// 3. The download fallback pattern: stream when the size is known
	// (HTTP Content-Length), buffer when it is not (chunked encoding).
	fmt.Println("\n3. Size-known / size-unknown fallback (HTTP download pattern):")

	chunked, err := os.Open(file.Name())
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	var fromChunked ValidatorRegistry
	if err = decodeSSZ(ds, &fromChunked, chunked, -1); err != nil {
		return fmt.Errorf("failed to decode without size: %w", err)
	}

	fmt.Printf("  chunked response (unknown size) decoded via buffered fallback: %d validators\n",
		len(fromChunked.Validators))

	// 4. Network-style streaming: encoder and decoder connected by a pipe.
	// Nothing is ever fully buffered — bytes flow from MarshalSSZWriter to
	// UnmarshalSSZReader as they are produced. The receiver only needs to
	// know the total size up front (an SSZ response's Content-Length).
	fmt.Println("\n4. io.Pipe — sender and receiver with no intermediate buffer:")

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		pipeWriter.CloseWithError(ds.MarshalSSZWriter(registry, pipeWriter))
	}()

	var received ValidatorRegistry
	if err = ds.UnmarshalSSZReader(&received, pipeReader, size); err != nil {
		return fmt.Errorf("failed to decode from pipe: %w", err)
	}

	fmt.Printf("  received %d validators over the pipe\n", len(received.Validators))

	// 5. Allocation comparison: read-then-unmarshal vs streamed decode.
	fmt.Println("\n5. Allocations, buffered vs streamed decode:")

	buffered, err := measureAllocs(func() error {
		data, readErr := os.ReadFile(file.Name())
		if readErr != nil {
			return fmt.Errorf("failed to read file: %w", readErr)
		}

		var target ValidatorRegistry

		return ds.UnmarshalSSZ(&target, data)
	})
	if err != nil {
		return err
	}

	streamed, err := measureAllocs(func() error {
		streamFile, openErr := os.Open(file.Name())
		if openErr != nil {
			return fmt.Errorf("failed to open file: %w", openErr)
		}
		defer streamFile.Close()

		var target ValidatorRegistry

		return ds.UnmarshalSSZReader(&target, streamFile, int(info.Size()))
	})
	if err != nil {
		return err
	}

	fmt.Printf("  buffered: %6.1f MB allocated (payload buffer + decoded struct)\n", float64(buffered)/(1<<20))
	fmt.Printf("  streamed: %6.1f MB allocated (decoded struct only)\n", float64(streamed)/(1<<20))
	fmt.Println()
	fmt.Println("Streaming trades ~1.3-2x CPU for the dropped payload buffer — worth it")
	fmt.Println("for beacon states, not for small messages. See docs/streaming.md.")

	return nil
}

// decodeSSZ is a fallback helper for beacon API downloads: stream directly
// when the payload size is known, buffer when it is not (SSZ offsets cannot
// be interpreted without the total size). For untrusted streams, wrap the
// reader in an io.LimitReader instead of using the unbounded buffered path.
func decodeSSZ(ds *dynssz.DynSsz, target any, data io.ReadCloser, size int64) error {
	defer func() { _ = data.Close() }()

	if size > 0 {
		return ds.UnmarshalSSZReader(target, data, int(size))
	}

	dataBytes, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("failed to read payload: %w", err)
	}

	return ds.UnmarshalSSZ(target, dataBytes)
}

func measureAllocs(work func() error) (uint64, error) {
	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	if err := work(); err != nil {
		return 0, err
	}

	runtime.ReadMemStats(&after)

	return after.TotalAlloc - before.TotalAlloc, nil
}

func buildRegistry(validatorCount int) *ValidatorRegistry {
	registry := &ValidatorRegistry{
		Slot:       123456,
		BlockRoots: make([][32]byte, 64),
		Validators: make([]Validator, 0, validatorCount),
		Balances:   make([]uint64, 0, validatorCount),
	}

	for i := 0; i < validatorCount; i++ {
		validator := Validator{
			EffectiveBalance: 32_000_000_000,
			ExitEpoch:        ^uint64(0),
		}
		validator.Pubkey[0] = byte(i)
		validator.Pubkey[1] = byte(i >> 8)

		registry.Validators = append(registry.Validators, validator)
		registry.Balances = append(registry.Balances, 32_000_000_000+uint64(i))
	}

	return registry
}
