// Package main demonstrates memory-efficient streaming operations with dynamic-ssz.
// This example shows how to process large SSZ structures without loading them entirely into memory.
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
)

// LargeData represents a structure that could be very large in practice.
// In real-world scenarios, this might be a BeaconState or similar large blockchain structure.
type LargeData struct {
	Version   uint64
	Timestamp uint64
	// Simulate large data with dynamic arrays
	Data      [][]byte  `ssz-size:"?,?" ssz-max:"1000,1024"`
	Metadata  []uint64  `ssz-max:"10000"`
	HashRoots [][32]byte `ssz-max:"8192"`
}

func main() {
	// Initialize dynssz with some specifications
	specs := map[string]any{
		"MAX_DATA_ITEMS": uint64(1000),
		"MAX_DATA_SIZE":  uint64(1024),
	}
	ds := dynssz.NewDynSsz(specs)

	// Create a sample large data structure
	largeData := createLargeData()

	// Example 1: Stream to file
	fmt.Println("Example 1: Streaming to file...")
	if err := streamToFile(ds, largeData); err != nil {
		log.Fatal("Stream to file failed:", err)
	}

	// Example 2: Stream from file
	fmt.Println("\nExample 2: Streaming from file...")
	loaded, err := streamFromFile(ds)
	if err != nil {
		log.Fatal("Stream from file failed:", err)
	}
	fmt.Printf("Loaded data with %d data items and %d metadata entries\n", len(loaded.Data), len(loaded.Metadata))

	// Example 3: Network streaming simulation
	fmt.Println("\nExample 3: Network streaming simulation...")
	if err := simulateNetworkStreaming(ds, largeData); err != nil {
		log.Fatal("Network streaming failed:", err)
	}

	// Example 4: Processing pipeline with streaming
	fmt.Println("\nExample 4: Processing pipeline...")
	if err := processingPipeline(ds); err != nil {
		log.Fatal("Processing pipeline failed:", err)
	}

	// Clean up
	os.Remove("large_data.ssz")
	fmt.Println("\nAll streaming examples completed successfully!")
}

// createLargeData creates a sample large data structure
func createLargeData() *LargeData {
	data := &LargeData{
		Version:   1,
		Timestamp: uint64(time.Now().Unix()),
		Data:      make([][]byte, 100),
		Metadata:  make([]uint64, 1000),
		HashRoots: make([][32]byte, 100),
	}

	// Fill with sample data
	for i := range data.Data {
		data.Data[i] = make([]byte, 256)
		for j := range data.Data[i] {
			data.Data[i][j] = byte(i + j)
		}
	}

	for i := range data.Metadata {
		data.Metadata[i] = uint64(i * 1000)
	}

	for i := range data.HashRoots {
		for j := range data.HashRoots[i] {
			data.HashRoots[i][j] = byte(i + j)
		}
	}

	return data
}

// streamToFile demonstrates streaming SSZ data directly to a file
func streamToFile(ds *dynssz.DynSsz, data *LargeData) error {
	file, err := os.Create("large_data.ssz")
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	// Calculate size for comparison
	size, err := ds.SizeSSZ(data)
	if err != nil {
		return fmt.Errorf("calculate size: %w", err)
	}
	fmt.Printf("Data size: %d bytes (%.2f MB)\n", size, float64(size)/(1024*1024))

	// Stream directly to file
	start := time.Now()
	if err := ds.MarshalSSZWriter(data, file); err != nil {
		return fmt.Errorf("marshal to writer: %w", err)
	}
	fmt.Printf("Streamed to file in %v\n", time.Since(start))

	// Verify file size
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	fmt.Printf("File size: %d bytes\n", info.Size())

	return nil
}

// streamFromFile demonstrates streaming SSZ data from a file
func streamFromFile(ds *dynssz.DynSsz) (*LargeData, error) {
	file, err := os.Open("large_data.ssz")
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	// Get file size
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// Stream from file
	start := time.Now()
	var data LargeData
	if err := ds.UnmarshalSSZReader(&data, file, info.Size()); err != nil {
		return nil, fmt.Errorf("unmarshal from reader: %w", err)
	}
	fmt.Printf("Streamed from file in %v\n", time.Since(start))

	return &data, nil
}

// simulateNetworkStreaming demonstrates streaming over a simulated network connection
func simulateNetworkStreaming(ds *dynssz.DynSsz, data *LargeData) error {
	// Create a pipe to simulate network connection
	reader, writer := io.Pipe()

	// Channel to signal completion
	done := make(chan error, 1)

	// Start server goroutine (receiver)
	go func() {
		defer reader.Close()
		var received LargeData
		err := ds.UnmarshalSSZReader(&received, reader, -1) // -1 means unknown size
		if err != nil {
			done <- fmt.Errorf("unmarshal from network: %w", err)
			return
		}
		fmt.Printf("Server received: %d data items, %d metadata entries\n", 
			len(received.Data), len(received.Metadata))
		done <- nil
	}()

	// Client sends data
	go func() {
		defer writer.Close()
		start := time.Now()
		if err := ds.MarshalSSZWriter(data, writer); err != nil {
			done <- fmt.Errorf("marshal to network: %w", err)
			return
		}
		fmt.Printf("Client sent data in %v\n", time.Since(start))
	}()

	// Wait for completion
	return <-done
}

// processingPipeline demonstrates a processing pipeline with streaming
func processingPipeline(ds *dynssz.DynSsz) error {
	// Create multiple data items
	items := make([]*LargeData, 5)
	for i := range items {
		items[i] = &LargeData{
			Version:   uint64(i + 1),
			Timestamp: uint64(time.Now().Unix()),
			Data:      [][]byte{[]byte(fmt.Sprintf("Item %d data", i))},
			Metadata:  []uint64{uint64(i * 100), uint64(i * 200)},
			HashRoots: make([][32]byte, 2),
		}
	}

	// Process pipeline: marshal -> transform -> unmarshal
	for i, item := range items {
		// Step 1: Marshal to buffer
		var buf bytes.Buffer
		if err := ds.MarshalSSZWriter(item, &buf); err != nil {
			return fmt.Errorf("marshal item %d: %w", i, err)
		}

		// Step 2: Simulate transformation (e.g., compression, encryption)
		// In real scenarios, this could be actual transformation
		transformed := buf.Bytes()

		// Step 3: Unmarshal from transformed data
		var processed LargeData
		reader := bytes.NewReader(transformed)
		if err := ds.UnmarshalSSZReader(&processed, reader, int64(len(transformed))); err != nil {
			return fmt.Errorf("unmarshal item %d: %w", i, err)
		}

		fmt.Printf("Processed item %d: version=%d, data_items=%d\n", 
			i, processed.Version, len(processed.Data))
	}

	return nil
}

// Example of a custom writer that counts bytes
type countingWriter struct {
	w     io.Writer
	count int64
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.count += int64(n)
	return
}

// Example showing how to use a custom writer
func streamWithCustomWriter(ds *dynssz.DynSsz, data *LargeData) error {
	// Create a counting writer that wraps stdout
	cw := &countingWriter{w: io.Discard} // Using Discard for example

	// Stream to counting writer
	if err := ds.MarshalSSZWriter(data, cw); err != nil {
		return fmt.Errorf("marshal with counting writer: %w", err)
	}

	fmt.Printf("Total bytes written: %d\n", cw.count)
	return nil
}

// Example of streaming with TCP (commented out as it requires actual network setup)
func streamOverTCP(ds *dynssz.DynSsz, data *LargeData, address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("dial tcp: %w", err)
	}
	defer conn.Close()

	// Set deadline for network operations
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Stream directly to TCP connection
	if err := ds.MarshalSSZWriter(data, conn); err != nil {
		return fmt.Errorf("marshal to tcp: %w", err)
	}

	return nil
}