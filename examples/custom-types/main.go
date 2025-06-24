// Package main demonstrates using dynamic-ssz with custom, non-Ethereum data structures.
// This example shows how the library can be used for any SSZ-compatible types with dynamic sizing.
package main

import (
	"fmt"
	"log"

	dynssz "github.com/pk910/dynamic-ssz"
)

// CustomMessage represents a general message structure with dynamic fields
type CustomMessage struct {
	ID        uint64
	Timestamp uint64
	Sender    [32]byte
	Data      []byte   `ssz-size:"1024" dynssz-size:"MAX_MESSAGE_SIZE"`
	Tags      [][]byte `ssz-size:"16,64" dynssz-size:"MAX_TAGS,MAX_TAG_LENGTH"`
}

// LogEntry represents a log entry with dynamic buffer size
type LogEntry struct {
	Level     uint32
	Timestamp uint64
	Module    [16]byte
	Message   []byte `ssz-size:"256" dynssz-size:"MAX_LOG_MESSAGE_SIZE"`
	Details   []byte `ssz-size:"512" dynssz-size:"LOG_BUFFER_SIZE"`
}

// NetworkPacket represents a network packet with variable payload
type NetworkPacket struct {
	Version  uint16
	Type     uint8
	Flags    uint8
	SourceID uint32
	TargetID uint32
	Sequence uint64
	Payload  []byte `ssz-size:"1500" dynssz-size:"MAX_PACKET_SIZE"`
	Checksum uint32
}

// Configuration represents application configuration with dynamic arrays
type Configuration struct {
	Version  uint32
	Servers  [][]byte `ssz-size:"10,128" dynssz-size:"MAX_SERVERS,SERVER_NAME_LENGTH"`
	Ports    []uint16 `ssz-size:"10" dynssz-size:"MAX_SERVERS"`
	Features []bool   `ssz-size:"64" dynssz-size:"MAX_FEATURES"`
	Timeouts []uint32 `ssz-size:"5" dynssz-size:"TIMEOUT_COUNT"`
}

func main() {
	fmt.Println("Dynamic SSZ Custom Types Example")
	fmt.Println("================================")

	// Define custom specifications for our application
	specs := map[string]any{
		"MAX_MESSAGE_SIZE":     uint64(2048),
		"MAX_TAGS":             uint64(8),
		"MAX_TAG_LENGTH":       uint64(32),
		"MAX_LOG_MESSAGE_SIZE": uint64(512),
		"LOG_BUFFER_SIZE":      uint64(1024),
		"MAX_PACKET_SIZE":      uint64(9000), // Jumbo frame
		"MAX_SERVERS":          uint64(5),
		"SERVER_NAME_LENGTH":   uint64(64),
		"MAX_FEATURES":         uint64(32),
		"TIMEOUT_COUNT":        uint64(3),
	}

	ds := dynssz.NewDynSsz(specs)

	// Example 1: Custom Message
	fmt.Println("\n1. Custom Message Example:")
	message := &CustomMessage{
		ID:        12345,
		Timestamp: 1640995200,
		Sender:    [32]byte{1, 2, 3, 4, 5},
		Data:      []byte("Hello, this is a custom message payload"),
		Tags:      [][]byte{[]byte("urgent"), []byte("system"), []byte("alert")},
	}

	msgData, err := ds.MarshalSSZ(message)
	if err != nil {
		log.Fatal("Failed to marshal message:", err)
	}
	fmt.Printf("Encoded message: %d bytes\n", len(msgData))

	var decodedMsg CustomMessage
	err = ds.UnmarshalSSZ(&decodedMsg, msgData)
	if err != nil {
		log.Fatal("Failed to unmarshal message:", err)
	}
	fmt.Printf("Decoded message ID: %d, tags: %v\n", decodedMsg.ID, decodedMsg.Tags)

	// Example 2: Log Entry
	fmt.Println("\n2. Log Entry Example:")
	logEntry := &LogEntry{
		Level:     2, // Info level
		Timestamp: 1640995300,
		Module:    [16]byte{'a', 'p', 'p', 'l', 'i', 'c', 'a', 't', 'i', 'o', 'n'},
		Message:   []byte("Application started successfully"),
		Details:   []byte(`{"version": "1.0.0", "config": "production"}`),
	}

	logData, err := ds.MarshalSSZ(logEntry)
	if err != nil {
		log.Fatal("Failed to marshal log entry:", err)
	}
	fmt.Printf("Encoded log entry: %d bytes\n", len(logData))

	var decodedLog LogEntry
	err = ds.UnmarshalSSZ(&decodedLog, logData)
	if err != nil {
		log.Fatal("Failed to unmarshal log entry:", err)
	}
	fmt.Printf("Decoded log message: %s\n", decodedLog.Message)

	// Example 3: Network Packet
	fmt.Println("\n3. Network Packet Example:")
	packet := &NetworkPacket{
		Version:  1,
		Type:     10, // Data packet
		Flags:    0x01,
		SourceID: 1001,
		TargetID: 2002,
		Sequence: 12345,
		Payload:  []byte("This is the packet payload data"),
		Checksum: 0xDEADBEEF,
	}

	packetData, err := ds.MarshalSSZ(packet)
	if err != nil {
		log.Fatal("Failed to marshal packet:", err)
	}
	fmt.Printf("Encoded packet: %d bytes\n", len(packetData))

	// Calculate hash tree root for integrity
	packetRoot, err := ds.HashTreeRoot(packet)
	if err != nil {
		log.Fatal("Failed to calculate packet root:", err)
	}
	fmt.Printf("Packet hash tree root: %x\n", packetRoot)

	// Example 4: Configuration
	fmt.Println("\n4. Configuration Example:")
	config := &Configuration{
		Version:  1,
		Servers:  [][]byte{[]byte("server1.example.com"), []byte("server2.example.com")},
		Ports:    []uint16{8080, 8443},
		Features: []bool{true, false, true, true, false},
		Timeouts: []uint32{5000, 10000, 30000},
	}

	configData, err := ds.MarshalSSZ(config)
	if err != nil {
		log.Fatal("Failed to marshal config:", err)
	}
	fmt.Printf("Encoded config: %d bytes\n", len(configData))

	var decodedConfig Configuration
	err = ds.UnmarshalSSZ(&decodedConfig, configData)
	if err != nil {
		log.Fatal("Failed to unmarshal config:", err)
	}
	fmt.Printf("Decoded config servers: %v\n", decodedConfig.Servers)

	// Example 5: Different specifications for different environments
	fmt.Println("\n5. Different Environment Specifications:")

	// Development environment with smaller limits
	devSpecs := map[string]any{
		"MAX_MESSAGE_SIZE":     uint64(512),
		"MAX_TAGS":             uint64(4),
		"MAX_TAG_LENGTH":       uint64(16),
		"MAX_LOG_MESSAGE_SIZE": uint64(128),
		"LOG_BUFFER_SIZE":      uint64(256),
		"MAX_PACKET_SIZE":      uint64(1500),
		"MAX_SERVERS":          uint64(2),
		"SERVER_NAME_LENGTH":   uint64(32),
		"MAX_FEATURES":         uint64(16),
		"TIMEOUT_COUNT":        uint64(2),
	}

	devDs := dynssz.NewDynSsz(devSpecs)

	// Same message structure, different size limits
	devMessage := &CustomMessage{
		ID:        54321,
		Timestamp: 1640995400,
		Sender:    [32]byte{9, 8, 7, 6, 5},
		Data:      []byte("Dev message"),
		Tags:      [][]byte{[]byte("dev"), []byte("test")},
	}

	devMsgData, err := devDs.MarshalSSZ(devMessage)
	if err != nil {
		log.Fatal("Failed to marshal dev message:", err)
	}
	fmt.Printf("Dev environment message: %d bytes\n", len(devMsgData))

	// Example 6: Size calculation for capacity planning
	fmt.Println("\n6. Size Calculation Example:")
	msgSize, err := ds.SizeSSZ(message)
	if err != nil {
		log.Fatal("Failed to calculate message size:", err)
	}
	fmt.Printf("Message SSZ size: %d bytes\n", msgSize)

	configSize, err := ds.SizeSSZ(config)
	if err != nil {
		log.Fatal("Failed to calculate config size:", err)
	}
	fmt.Printf("Config SSZ size: %d bytes\n", configSize)

	fmt.Println("\nCustom types example completed successfully!")
	fmt.Println("This demonstrates how dynamic-ssz can be used with any SSZ-compatible")
	fmt.Println("data structures, not just Ethereum types.")
}
