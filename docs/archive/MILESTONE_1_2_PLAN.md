# Milestone 1.2: Network Layer - Implementation Plan

**Start Date**: January 7, 2025
**Target Duration**: 2-3 sessions
**Status**: 🚧 **PLANNING**

---

## Overview

Build a high-performance network layer that enables client-server communication with our storage engine.

---

## Objectives

### Primary Goals

1. ✅ Design efficient binary protocol
2. ✅ Implement TCP server with concurrent connection handling
3. ✅ Create request/response handlers
4. ✅ Build connection pooling
5. ✅ Implement Go client SDK
6. ✅ Comprehensive testing
7. ✅ Performance benchmarking

### Success Criteria

- [ ] TCP server handles 10K+ concurrent connections
- [ ] Request latency < 1ms (excluding storage)
- [ ] Zero-copy where possible
- [ ] Graceful connection handling
- [ ] Client SDK is intuitive
- [ ] 80%+ test coverage
- [ ] Comprehensive benchmarks

---

## Architecture

### Network Stack

```
Client Application
       ↓
Client Library (SDK)
       ↓
TCP Connection
       ↓
Protocol Handler
       ↓
Request Router
       ↓
Storage Engine
```

### Protocol Design

**Binary Protocol Format**:

```
Request:
[Length: 4 bytes][RequestID: 8 bytes][Type: 1 byte][Payload: N bytes]

Response:
[Length: 4 bytes][RequestID: 8 bytes][Status: 1 byte][Payload: N bytes]
```

**Request Types**:
- `0x01` - Produce (append messages)
- `0x02` - Fetch (read messages)
- `0x03` - GetOffset (get high water mark)
- `0x04` - CreateTopic
- `0x05` - DeleteTopic
- `0x06` - ListTopics
- `0x07` - HealthCheck

---

## Implementation Plan

### Phase 1: Protocol Definition (Session 1)

**Files to Create**:
1. `pkg/protocol/types.go` - Protocol types and constants
2. `pkg/protocol/codec.go` - Encode/decode logic
3. `pkg/protocol/requests.go` - Request structures
4. `pkg/protocol/responses.go` - Response structures

**Tasks**:
- [ ] Define request/response structures
- [ ] Implement binary serialization
- [ ] Add CRC32 checksums for integrity
- [ ] Write comprehensive tests
- [ ] Benchmark encode/decode performance

**Estimated Time**: 2 hours

### Phase 2: TCP Server (Session 1)

**Files to Create**:
1. `pkg/server/server.go` - TCP server implementation
2. `pkg/server/connection.go` - Connection lifecycle management
3. `pkg/server/handler.go` - Request handler

**Tasks**:
- [ ] Implement TCP listener
- [ ] Handle concurrent connections (goroutine per connection)
- [ ] Graceful shutdown support
- [ ] Connection timeout handling
- [ ] Request routing to storage engine
- [ ] Error handling and logging

**Estimated Time**: 3 hours

### Phase 3: Client Library (Session 2)

**Files to Create**:
1. `pkg/client/client.go` - Client SDK
2. `pkg/client/connection.go` - Connection management
3. `pkg/client/producer.go` - Producer API
4. `pkg/client/consumer.go` - Consumer API

**Tasks**:
- [ ] Implement connection pooling
- [ ] Synchronous request/response
- [ ] Asynchronous batching for producers
- [ ] Retry logic with backoff
- [ ] Client-side error handling
- [ ] Intuitive API design

**Estimated Time**: 3 hours

### Phase 4: Testing & Benchmarking (Session 2-3)

**Files to Create**:
1. `pkg/protocol/codec_test.go`
2. `pkg/server/server_test.go`
3. `pkg/client/client_test.go`
4. `integration_test.go` - End-to-end tests

**Tasks**:
- [ ] Unit tests for all components
- [ ] Integration tests (client → server → storage)
- [ ] Benchmark encode/decode
- [ ] Benchmark request/response latency
- [ ] Load testing (concurrent connections)
- [ ] Stress testing (sustained throughput)

**Estimated Time**: 2 hours

---

## Detailed Design

### Protocol Specification

#### Request Format

```
+----------------+
| Length (4B)    | Total message length (excluding this field)
+----------------+
| RequestID (8B) | Unique request identifier
+----------------+
| Type (1B)      | Request type code
+----------------+
| Version (1B)   | Protocol version
+----------------+
| Flags (2B)     | Request flags
+----------------+
| Payload (N)    | Type-specific payload
+----------------+
| CRC32 (4B)     | Checksum of entire message
+----------------+
```

#### Produce Request Payload

```
+----------------+
| PartitionID (4B)
+----------------+
| NumMessages (4B)
+----------------+
| Message 1      |
+----------------+
| Message 2      |
+----------------+
| ...            |
+----------------+
```

#### Fetch Request Payload

```
+----------------+
| PartitionID (4B)
+----------------+
| Offset (8B)    | Starting offset
+----------------+
| MaxBytes (4B)  | Maximum bytes to fetch
+----------------+
```

#### Response Format

```
+----------------+
| Length (4B)    |
+----------------+
| RequestID (8B) | Matches request
+----------------+
| Status (1B)    | 0=Success, 1=Error, 2=PartialSuccess
+----------------+
| ErrorCode (2B) | Error code if status != 0
+----------------+
| Payload (N)    |
+----------------+
| CRC32 (4B)     |
+----------------+
```

### Error Codes

```go
const (
    ErrNone              = 0
    ErrUnknownRequest    = 1
    ErrInvalidRequest    = 2
    ErrOffsetOutOfRange  = 3
    ErrCorruptMessage    = 4
    ErrPartitionNotFound = 5
    ErrRequestTimeout    = 6
    ErrStorageError      = 7
)
```

### TCP Server Design

**Key Features**:
- One goroutine per connection
- Non-blocking I/O where possible
- Graceful shutdown (drain connections)
- Connection pooling reuse
- Read/write timeouts

**Pseudo-code**:

```go
func (s *Server) Start() error {
    listener := net.Listen("tcp", s.addr)

    for {
        conn, err := listener.Accept()
        if err != nil {
            // Handle shutdown
            continue
        }

        go s.handleConnection(conn)
    }
}

func (s *Server) handleConnection(conn net.Conn) {
    defer conn.Close()

    for {
        // Read request
        req, err := s.codec.DecodeRequest(conn)
        if err != nil {
            break
        }

        // Route to handler
        resp := s.handler.Handle(req)

        // Write response
        s.codec.EncodeResponse(conn, resp)
    }
}
```

### Client Library Design

**API Goals**:
- Simple and intuitive
- Support both sync and async operations
- Automatic retries
- Connection pooling
- Batch optimization

**Example Usage**:

```go
// Create client
client, err := streambus.NewClient("localhost:9092")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Produce messages (async with batching)
producer := client.Producer()
err = producer.Produce(ctx, &Message{
    Key:   []byte("key"),
    Value: []byte("value"),
})

// Consume messages
consumer := client.Consumer()
messages, err := consumer.Fetch(ctx, partition, offset, maxBytes)
```

---

## Performance Targets

### Latency

| Operation | Target | Stretch Goal |
|-----------|--------|--------------|
| Encode Request | < 1µs | < 500ns |
| Decode Request | < 1µs | < 500ns |
| Network Round-Trip | < 1ms | < 500µs |
| Total Produce Latency | < 2ms | < 1ms |

### Throughput

| Metric | Target | Stretch Goal |
|--------|--------|--------------|
| Concurrent Connections | 10K | 50K |
| Requests/sec (single conn) | 100K | 500K |
| Requests/sec (server) | 1M | 5M |
| Bandwidth | 1 GB/s | 5 GB/s |

### Resource Usage

| Metric | Target |
|--------|--------|
| Memory per connection | < 4KB |
| CPU per connection | < 0.1% |
| GC pause | < 1ms |

---

## Testing Strategy

### Unit Tests

1. **Protocol Tests**:
   - Encode/decode correctness
   - Checksum validation
   - Malformed message handling
   - Version compatibility

2. **Server Tests**:
   - Connection handling
   - Request routing
   - Error handling
   - Graceful shutdown

3. **Client Tests**:
   - Connection pooling
   - Retry logic
   - Timeout handling
   - API usage

### Integration Tests

1. **End-to-End**:
   - Client → Server → Storage
   - Produce and fetch messages
   - Error propagation
   - Connection recovery

2. **Load Tests**:
   - 10K concurrent connections
   - Sustained 1M req/s
   - Memory stability
   - CPU efficiency

### Benchmarks

```
BenchmarkEncode
BenchmarkDecode
BenchmarkServerRequestLatency
BenchmarkClientRoundTrip
BenchmarkConcurrentConnections
BenchmarkSustainedThroughput
```

---

## File Structure

```
pkg/
├── protocol/
│   ├── types.go          - Protocol types and constants
│   ├── codec.go          - Encode/decode
│   ├── requests.go       - Request structures
│   ├── responses.go      - Response structures
│   ├── codec_test.go     - Protocol tests
│   └── errors.go         - Error codes
├── server/
│   ├── server.go         - TCP server
│   ├── connection.go     - Connection management
│   ├── handler.go        - Request handler
│   ├── config.go         - Server configuration
│   └── server_test.go    - Server tests
├── client/
│   ├── client.go         - Client SDK
│   ├── connection.go     - Connection pool
│   ├── producer.go       - Producer API
│   ├── consumer.go       - Consumer API
│   ├── config.go         - Client configuration
│   └── client_test.go    - Client tests
└── integration/
    └── integration_test.go - End-to-end tests
```

---

## Dependencies

### Standard Library
- `net` - TCP networking
- `encoding/binary` - Binary serialization
- `hash/crc32` - Checksums
- `context` - Request context
- `sync` - Concurrency primitives

### Third-Party (Minimal)
- None required! Pure Go implementation

---

## Risks & Mitigations

### Risk 1: Connection Scaling
**Risk**: Server may not handle 10K+ connections
**Mitigation**:
- Use connection pooling
- Implement rate limiting
- Test under load early

### Risk 2: Protocol Changes
**Risk**: Need to support multiple protocol versions
**Mitigation**:
- Include version field in protocol
- Design for backward compatibility
- Document breaking changes

### Risk 3: Performance
**Risk**: Network overhead impacts latency
**Mitigation**:
- Use binary protocol (not JSON/text)
- Implement zero-copy where possible
- Benchmark continuously

---

## Success Metrics

### Code Quality
- [ ] 80%+ test coverage
- [ ] Zero known bugs
- [ ] All tests passing
- [ ] Benchmarks meet targets

### Performance
- [ ] < 1ms request latency
- [ ] 10K+ concurrent connections
- [ ] 1M+ req/s throughput

### Usability
- [ ] Intuitive client API
- [ ] Good error messages
- [ ] Comprehensive documentation

---

## Timeline

### Session 1 (3-4 hours)
- Design protocol
- Implement codec
- Build TCP server
- Basic request handling

### Session 2 (3-4 hours)
- Implement client library
- Connection pooling
- Producer/Consumer APIs
- Unit tests

### Session 3 (2-3 hours)
- Integration tests
- Load testing
- Performance tuning
- Documentation

**Total**: 8-11 hours estimated

---

## Next Steps

### Immediate Actions

1. Create `pkg/protocol/` directory structure
2. Define protocol types and constants
3. Implement binary codec
4. Write protocol tests

### Questions to Answer

- Should we use framing (length prefix) or delimiters?
  - **Decision**: Length prefix (more efficient)
- Binary vs text protocol?
  - **Decision**: Binary (better performance)
- Connection pooling strategy?
  - **Decision**: Pool per server, round-robin

---

## References

- Kafka Protocol: https://kafka.apache.org/protocol
- gRPC Design: https://grpc.io/docs/
- TCP Best Practices: https://www.kernel.org/doc/html/latest/networking/

---

**Ready to start building!** 🚀

Let's begin with Phase 1: Protocol Definition
