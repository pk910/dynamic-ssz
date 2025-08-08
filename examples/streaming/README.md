# Streaming Example

This example demonstrates the memory-efficient streaming capabilities of dynamic-ssz for handling large SSZ structures.

## Features Demonstrated

1. **File Streaming**: Write and read SSZ data directly to/from files without loading entire structures into memory
2. **Network Streaming**: Simulate streaming over network connections using io.Pipe
3. **Processing Pipeline**: Chain streaming operations for data transformation
4. **Custom Writers**: Use custom io.Writer implementations for monitoring or transformation

## Running the Example

```bash
go run main.go
```

## Key Benefits

The streaming approach provides several advantages:

- **Constant Memory Usage**: Process gigabyte-sized structures with only megabytes of RAM
- **Direct I/O**: Data flows directly to/from the destination without intermediate buffers
- **Scalability**: Handle structures larger than available memory
- **Network Efficiency**: Start transmitting data before complete serialization

## Use Cases

This streaming functionality is particularly useful for:

- Processing large blockchain state files
- Streaming SSZ data over network connections
- Efficient file I/O for SSZ structures
- Memory-constrained environments
- Real-time processing of SSZ data streams

## Code Highlights

### Streaming to File
```go
file, _ := os.Create("data.ssz")
defer file.Close()
err := ds.MarshalSSZWriter(data, file)
```

### Streaming from File
```go
file, _ := os.Open("data.ssz")
defer file.Close()
info, _ := file.Stat()
err := ds.UnmarshalSSZReader(&data, file, info.Size())
```

### Network Streaming
```go
conn, _ := net.Dial("tcp", "server:8080")
defer conn.Close()
err := ds.MarshalSSZWriter(data, conn)
```