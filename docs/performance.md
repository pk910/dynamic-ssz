# Performance Guide

This document provides comprehensive guidance on optimizing performance when using the dynamic-ssz library.

## Performance Philosophy

Dynamic-ssz employs a hybrid approach that balances flexibility with performance:

- **Static types**: Automatically uses fastssz for optimal performance
- **Dynamic types**: Uses reflection-based processing for flexibility
- **Caching**: Extensive type caching reduces overhead
- **Buffer management**: Efficient memory usage patterns

## Performance Characteristics

### Benchmark Results

Based on testing with BeaconBlocks and BeaconStates from kurtosis testnets:

#### Mainnet Preset

**BeaconBlock (10,000 operations):**
- fastssz only: [8ms unmarshal / 3ms marshal / 88ms hash]
- dynssz only: [27ms unmarshal / 12ms marshal / 63ms hash]
- **dynssz + fastssz: [8ms unmarshal / 3ms marshal / 64ms hash]** ← Recommended

**BeaconState (10,000 operations):**
- fastssz only: [5.8s unmarshal / 5.0s marshal / 73s hash]
- dynssz only: [22.5s unmarshal / 12.3s marshal / 40s hash]
- **dynssz + fastssz: [5.7s unmarshal / 4.9s marshal / 37s hash]** ← Recommended

#### Minimal Preset

**BeaconBlock (10,000 operations):**
- fastssz only: Failed (unmarshal error)
- dynssz only: [44ms unmarshal / 29ms marshal / 90ms hash]
- **dynssz + fastssz: [22ms unmarshal / 13ms marshal / 151ms hash]** ← Only option

**BeaconState (10,000 operations):**
- fastssz only: Failed (unmarshal error)
- dynssz only: [796ms unmarshal / 407ms marshal / 1816ms hash]
- **dynssz + fastssz: [459ms unmarshal / 244ms marshal / 4712ms hash]** ← Only option

## Optimization Strategies

### 1. Instance Reuse

**Always reuse DynSsz instances for maximum performance:**

```go
// ❌ Bad: Creates new instance every time
func processBlock(block *phase0.SignedBeaconBlock) {
    ds := dynssz.NewDynSsz(specs) // Expensive!
    data, _ := ds.MarshalSSZ(block)
}

// ✅ Good: Reuse instance
type Processor struct {
    dynSsz *dynssz.DynSsz
}

func (p *Processor) processBlock(block *phase0.SignedBeaconBlock) {
    data, _ := p.dynSsz.MarshalSSZ(block)
}
```

### 2. Buffer Reuse

**Use MarshalSSZTo with pre-allocated buffers:**

```go
// ❌ Bad: Allocates new buffer each time
func processBlocks(ds *dynssz.DynSsz, blocks []Block) {
    for _, block := range blocks {
        data, _ := ds.MarshalSSZ(block) // New allocation each time
    }
}

// ✅ Good: Reuse buffer
func processBlocks(ds *dynssz.DynSsz, blocks []Block) {
    buf := make([]byte, 0, 1024*1024) // 1MB buffer
    for _, block := range blocks {
        buf = buf[:0] // Reset length, keep capacity
        data, _ := ds.MarshalSSZTo(block, buf)
        // Process data...
    }
}
```

### 3. Size Calculation Optimization

**Pre-calculate sizes when possible:**

```go
// ✅ Good: Calculate size once for batch operations
func batchProcess(ds *dynssz.DynSsz, items []Item) {
    // Calculate expected size for buffer allocation
    totalSize := 0
    for _, item := range items {
        size, _ := ds.SizeSSZ(item)
        totalSize += size
    }
    
    buf := make([]byte, 0, totalSize)
    for _, item := range items {
        buf, _ = ds.MarshalSSZTo(item, buf)
    }
}
```

### 4. Type Cache Optimization

**Leverage type cache for repeated operations:**

```go
// ✅ Good: Type cache is automatically used
type Service struct {
    dynSsz *dynssz.DynSsz
}

func (s *Service) processMultipleBlocks(blocks []*phase0.SignedBeaconBlock) {
    // First block builds type cache
    // Subsequent blocks reuse cached type information
    for _, block := range blocks {
        data, _ := s.dynSsz.MarshalSSZ(block)
        // Process...
    }
}
```

## Memory Management

### 1. Buffer Sizing

**Choose appropriate buffer sizes:**

```go
// Small objects (< 1KB)
buf := make([]byte, 0, 1024)

// Medium objects (< 100KB)
buf := make([]byte, 0, 100*1024)

// Large objects (BeaconState, etc.)
buf := make([]byte, 0, 2*1024*1024) // 2MB
```

### 2. Memory Pooling

**Use sync.Pool for high-frequency operations:**

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 1024*1024)
    },
}

func processWithPool(ds *dynssz.DynSsz, block Block) {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf[:0])
    
    data, _ := ds.MarshalSSZTo(block, buf)
    // Process data...
}
```

### 3. Avoid Memory Leaks

**Be careful with large slices:**

```go
// ❌ Bad: May cause memory leak
func process(ds *dynssz.DynSsz, largeData []byte) []byte {
    return largeData[100:200] // Still references entire slice
}

// ✅ Good: Copy when necessary
func process(ds *dynssz.DynSsz, largeData []byte) []byte {
    result := make([]byte, 100)
    copy(result, largeData[100:200])
    return result
}
```

## Configuration Tuning

### 1. Disable Unnecessary Features

```go
ds := dynssz.NewDynSsz(specs)

// For maximum performance, disable verbose logging
ds.Verbose = false

// Only disable fastssz if you need pure dynamic behavior
// ds.NoFastSsz = true // Usually not recommended

// Only disable fast hashing if you have specific requirements
// ds.NoFastHash = true // Usually not recommended
```

## Profiling and Monitoring

### 1. CPU Profiling

```go
import _ "net/http/pprof"
import "net/http"

// Add to your main function
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

// Then profile with:
// go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

### 2. Memory Profiling

```go
// Check memory usage
func checkMemory() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    fmt.Printf("Alloc = %d KB", bToKb(m.Alloc))
    fmt.Printf("TotalAlloc = %d KB", bToKb(m.TotalAlloc))
    fmt.Printf("Sys = %d KB", bToKb(m.Sys))
}

func bToKb(b uint64) uint64 {
    return b / 1024
}
```

### 3. Benchmarking

```go
func BenchmarkMarshalSSZ(b *testing.B) {
    ds := dynssz.NewDynSsz(specs)
    block := createTestBlock()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := ds.MarshalSSZ(block)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkMarshalSSZTo(b *testing.B) {
    ds := dynssz.NewDynSsz(specs)
    block := createTestBlock()
    buf := make([]byte, 0, 1024)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        buf = buf[:0]
        _, err := ds.MarshalSSZTo(block, buf)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## Performance Patterns

### 1. Batch Processing Pattern

```go
type BatchProcessor struct {
    dynSsz *dynssz.DynSsz
    buffer []byte
}

func (bp *BatchProcessor) ProcessBatch(items []interface{}) error {
    for _, item := range items {
        bp.buffer = bp.buffer[:0]
        data, err := bp.dynSsz.MarshalSSZTo(item, bp.buffer)
        if err != nil {
            return err
        }
        // Process data...
    }
    return nil
}
```

### 2. Worker Pool Pattern

```go
type Worker struct {
    dynSsz *dynssz.DynSsz
    buffer []byte
}

func (w *Worker) Process(item interface{}) ([]byte, error) {
    w.buffer = w.buffer[:0]
    return w.dynSsz.MarshalSSZTo(item, w.buffer)
}

func CreateWorkerPool(size int, specs map[string]any) []*Worker {
    workers := make([]*Worker, size)
    for i := range workers {
        workers[i] = &Worker{
            dynSsz: dynssz.NewDynSsz(specs),
            buffer: make([]byte, 0, 1024*1024),
        }
    }
    return workers
}
```

### 3. Streaming Pattern

```go
type StreamProcessor struct {
    dynSsz *dynssz.DynSsz
    writer io.Writer
    buffer []byte
}

func (sp *StreamProcessor) ProcessStream(items <-chan interface{}) error {
    for item := range items {
        sp.buffer = sp.buffer[:0]
        data, err := sp.dynSsz.MarshalSSZTo(item, sp.buffer)
        if err != nil {
            return err
        }
        
        if _, err := sp.writer.Write(data); err != nil {
            return err
        }
    }
    return nil
}
```

### 4. Memory-Efficient Streaming Pattern

**New streaming methods eliminate memory overhead for large structures:**

```go
// ❌ Traditional approach: Entire data in memory
func saveState(ds *dynssz.DynSsz, state *phase0.BeaconState, filename string) error {
    data, err := ds.MarshalSSZ(state) // Allocates full size in memory
    if err != nil {
        return err
    }
    return os.WriteFile(filename, data, 0644)
}

// ✅ Streaming approach: Constant memory usage
func saveStateStreaming(ds *dynssz.DynSsz, state *phase0.BeaconState, filename string) error {
    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // Streams directly to disk with minimal memory overhead
    return ds.MarshalSSZWriter(state, file)
}

// ✅ Network streaming: No intermediate buffers
func sendStateOverNetwork(ds *dynssz.DynSsz, state *phase0.BeaconState, conn net.Conn) error {
    // Streams directly to network connection
    return ds.MarshalSSZWriter(state, conn)
}

// ✅ Reading large files efficiently
func loadStateStreaming(ds *dynssz.DynSsz, filename string) (*phase0.BeaconState, error) {
    file, err := os.Open(filename)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    info, err := file.Stat()
    if err != nil {
        return nil, err
    }
    
    var state phase0.BeaconState
    // Reads incrementally without loading entire file into memory
    err = ds.UnmarshalSSZReader(&state, file, info.Size())
    return &state, err
}
```

**Benefits of streaming methods:**
- **Constant memory usage**: Process gigabyte-sized structures with megabytes of RAM
- **Improved latency**: Start transmitting data before complete serialization
- **Better I/O efficiency**: Direct writes to destination without intermediate buffers
- **Scalability**: Handle structures larger than available memory

## Common Performance Pitfalls

### 1. Creating New Instances

```go
// ❌ Don't do this in hot paths
for _, block := range blocks {
    ds := dynssz.NewDynSsz(specs) // Creates new type cache each time!
    data, _ := ds.MarshalSSZ(block)
}
```

### 2. Not Reusing Buffers

```go
// ❌ Allocates new buffer each time
for _, block := range blocks {
    data, _ := ds.MarshalSSZ(block) // New allocation
}
```

### 3. Ignoring Size Hints

```go
// ❌ Buffer too small, causes reallocations
buf := make([]byte, 0, 10) // Too small for typical blocks
for _, block := range blocks {
    buf, _ = ds.MarshalSSZTo(block, buf[:0])
}
```

## Monitoring Performance

### Key Metrics to Track

1. **Throughput**: Operations per second
2. **Latency**: Time per operation
3. **Memory usage**: Allocation rate and GC pressure
4. **Cache hit rate**: Type cache effectiveness

### Example Monitoring

```go
type PerformanceMonitor struct {
    totalOps    int64
    totalTime   time.Duration
    startTime   time.Time
}

func (pm *PerformanceMonitor) StartOperation() func() {
    start := time.Now()
    return func() {
        atomic.AddInt64(&pm.totalOps, 1)
        pm.totalTime += time.Since(start)
    }
}

func (pm *PerformanceMonitor) Stats() (float64, time.Duration) {
    ops := atomic.LoadInt64(&pm.totalOps)
    elapsed := time.Since(pm.startTime)
    
    throughput := float64(ops) / elapsed.Seconds()
    avgLatency := pm.totalTime / time.Duration(ops)
    
    return throughput, avgLatency
}
```

By following these performance guidelines, you can achieve optimal performance with dynamic-ssz while maintaining the flexibility it provides for handling different Ethereum presets and custom specifications.