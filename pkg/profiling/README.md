# StreamBus Performance Profiling

Runtime profiling support using Go's pprof package.

## Features

- CPU profiling
- Memory profiling (heap, allocs)
- Goroutine profiling
- Block profiling
- Mutex profiling
- Custom runtime statistics
- HTTP endpoints for live profiling
- Programmatic GC control

## Quick Start

### Enable Profiling

```go
import "github.com/shawntherrien/streambus/pkg/profiling"

// Create profiler
profiler := profiling.New(&profiling.Config{
    Enabled:               true,
    ListenAddr:            ":6060",
    EnableCPUProfiling:    true,
    EnableMemoryProfiling: true,
    EnableBlockProfiling:  true,
    EnableMutexProfiling:  true,
})

// Start profiling server
if err := profiler.Start(); err != nil {
    log.Fatal(err)
}
defer profiler.Stop()
```

### Configuration Options

```go
type Config struct {
    Enabled    bool   // Enable/disable profiling
    ListenAddr string // HTTP server address (default: :6060)

    EnableCPUProfiling    bool // Enable CPU profiling
    EnableMemoryProfiling bool // Enable memory profiling
    EnableBlockProfiling  bool // Enable block profiling
    EnableMutexProfiling  bool // Enable mutex profiling

    BlockRate     int // Block profiling rate (default: 1)
    MutexFraction int // Mutex profiling fraction (default: 1)
}
```

## Available Endpoints

### Standard pprof Endpoints

- **http://localhost:6060/debug/pprof/** - Index of available profiles
- **http://localhost:6060/debug/pprof/cmdline** - Command line arguments
- **http://localhost:6060/debug/pprof/profile** - CPU profile (30s default)
- **http://localhost:6060/debug/pprof/symbol** - Symbol resolution
- **http://localhost:6060/debug/pprof/trace** - Execution trace

### Profile Types

- **http://localhost:6060/debug/pprof/heap** - Memory allocations
- **http://localhost:6060/debug/pprof/goroutine** - All goroutines
- **http://localhost:6060/debug/pprof/threadcreate** - Thread creation
- **http://localhost:6060/debug/pprof/block** - Blocking operations
- **http://localhost:6060/debug/pprof/mutex** - Mutex contention
- **http://localhost:6060/debug/pprof/allocs** - All memory allocations

### Custom Endpoints

- **http://localhost:6060/debug/stats** - Runtime statistics
- **http://localhost:6060/debug/gc** - Trigger garbage collection
- **http://localhost:6060/debug/freeOSMemory** - Return memory to OS

## Usage Examples

### CPU Profiling

```bash
# Capture 30-second CPU profile
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze with pprof
go tool pprof cpu.prof

# Commands in pprof:
(pprof) top10        # Show top 10 functions
(pprof) list funcName # Show source code
(pprof) web          # Open browser visualization
```

### Memory Profiling

```bash
# Capture heap profile
curl http://localhost:6060/debug/pprof/heap > heap.prof

# Analyze memory usage
go tool pprof heap.prof

# In pprof:
(pprof) top10                  # Top memory consumers
(pprof) list funcName          # Function details
(pprof) png > memory-graph.png # Generate graph
```

### Goroutine Profiling

```bash
# Get goroutine dump (text format)
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt

# Or binary format for analysis
curl http://localhost:6060/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
```

### Block Profiling

```bash
# Capture blocking profile
curl http://localhost:6060/debug/pprof/block > block.prof

# Analyze blocking operations
go tool pprof block.prof
(pprof) top10  # Top blocking sources
```

### Mutex Profiling

```bash
# Capture mutex contention profile
curl http://localhost:6060/debug/pprof/mutex > mutex.prof

# Analyze contention
go tool pprof mutex.prof
(pprof) top10  # Top contended mutexes
```

## Programmatic Usage

### Get Memory Statistics

```go
stats := profiling.GetMemStats()
fmt.Printf("Allocated: %d MB\n", stats.Alloc/1024/1024)
fmt.Printf("Total Allocated: %d MB\n", stats.TotalAlloc/1024/1024)
fmt.Printf("System Memory: %d MB\n", stats.Sys/1024/1024)
fmt.Printf("GC Runs: %d\n", stats.NumGC)
```

### Monitor Goroutines

```go
count := profiling.GetGoroutineCount()
if count > 10000 {
    log.Printf("Warning: High goroutine count: %d", count)
}
```

### Force Garbage Collection

```go
// Before taking a memory snapshot
profiling.TriggerGC()
time.Sleep(time.Second) // Allow GC to complete
stats := profiling.GetMemStats()
```

### Free OS Memory

```go
// Return unused memory to OS
profiling.FreeOSMemory()
```

## Visual Analysis

### Using pprof Web UI

```bash
# Start web server
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/heap

# Opens browser with:
# - Flame graphs
# - Call graphs
# - Source code view
# - Top functions
```

### Generate Visualizations

```bash
# Call graph (requires graphviz)
go tool pprof -png http://localhost:6060/debug/pprof/heap > heap.png

# Flame graph
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile
```

## Production Use

### Security Considerations

1. **Do not expose publicly**: Bind to localhost or internal network only
2. **Use authentication**: Add auth middleware if exposed
3. **Limit access**: Use firewall rules to restrict access
4. **Monitor overhead**: Profile collection has performance impact

### Recommended Configuration

```go
// Production profiling (minimal overhead)
&profiling.Config{
    Enabled:              true,
    ListenAddr:           "localhost:6060",  // Localhost only
    EnableCPUProfiling:   false,  // Enable on-demand
    EnableMemoryProfiling: true,
    EnableBlockProfiling:  false,
    EnableMutexProfiling:  false,
}
```

### On-Demand Profiling

Instead of always-on profiling, enable it temporarily:

```bash
# Signal handler to enable profiling
kill -USR1 <pid>  # Enable profiling
kill -USR2 <pid>  # Disable profiling
```

## Troubleshooting

### High Memory Usage

```bash
# 1. Check current memory
curl http://localhost:6060/debug/stats

# 2. Capture heap profile
curl http://localhost:6060/debug/pprof/heap > heap.prof

# 3. Analyze allocations
go tool pprof heap.prof
(pprof) top10
(pprof) list <function-name>

# 4. Check for goroutine leaks
curl http://localhost:6060/debug/pprof/goroutine?debug=2 | grep -c "goroutine"
```

### High CPU Usage

```bash
# 1. Capture CPU profile
curl "http://localhost:6060/debug/pprof/profile?seconds=30" > cpu.prof

# 2. Find hotspots
go tool pprof cpu.prof
(pprof) top20

# 3. Examine specific function
(pprof) list <function-name>
```

### Goroutine Leaks

```bash
# 1. Check goroutine count over time
for i in {1..10}; do
  count=$(curl -s http://localhost:6060/debug/pprof/goroutine?debug=2 | grep -c "goroutine")
  echo "$(date): $count goroutines"
  sleep 60
done

# 2. Analyze goroutine profiles
curl http://localhost:6060/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
(pprof) top10
```

### Mutex Contention

```bash
# 1. Check if mutex profiling is enabled
# (should be enabled in config)

# 2. Capture mutex profile
curl http://localhost:6060/debug/pprof/mutex > mutex.prof

# 3. Find contended mutexes
go tool pprof mutex.prof
(pprof) top10
(pprof) list <function-name>
```

## Integration with Monitoring

### Prometheus Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

// Export memory metrics
gauge := prometheus.NewGauge(prometheus.GaugeOpts{
    Name: "memory_allocated_bytes",
})

go func() {
    for {
        stats := profiling.GetMemStats()
        gauge.Set(float64(stats.Alloc))
        time.Sleep(10 * time.Second)
    }
}()
```

### Automated Alerts

```bash
# Script to monitor and alert
#!/bin/bash
while true; do
  goroutines=$(curl -s http://localhost:6060/debug/pprof/goroutine?debug=2 | grep -c "goroutine")
  if [ $goroutines -gt 10000 ]; then
    echo "ALERT: High goroutine count: $goroutines"
    # Send alert via PagerDuty, Slack, etc.
  fi
  sleep 60
done
```

## Best Practices

1. **Profile in production-like environments**: Staging should match production
2. **Collect baselines**: Establish normal performance profiles
3. **Compare before/after**: Use `benchstat` to compare profiles
4. **Profile under load**: Capture profiles during peak traffic
5. **Save profiles**: Archive profiles for historical analysis
6. **Focus on deltas**: Look for changes in profiles over time
7. **Profile duration**: 30 seconds is usually sufficient for CPU
8. **Memory snapshots**: Take heap profiles at different times

## Examples

See [examples/profiling](../../examples/profiling) for complete examples:
- Basic profiling setup
- Memory leak detection
- Goroutine leak investigation
- CPU hotspot analysis
- Production profiling patterns

## References

- [Go pprof Documentation](https://golang.org/pkg/net/http/pprof/)
- [Profiling Go Programs](https://blog.golang.org/pprof)
- [runtime/pprof](https://golang.org/pkg/runtime/pprof/)
- [Performance Tuning Guide](https://github.com/golang/go/wiki/Performance)
