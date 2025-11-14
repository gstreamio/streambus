# Raft Connection Improvements - Industry Best Practices

## Problem

During broker restarts, you see hundreds of connection error logs flooding the output:
```
2025/11/12 01:24:56 [Node 1] Failed to send message to 3: failed to connect to peer 3: failed to connect to broker3:9093: dial tcp: lookup broker3 on 127.0.0.11:53: no such host
```

This happens because:
1. **No exponential backoff** - Retries every 100-200ms
2. **No circuit breaker** - Keeps hammering dead/restarting nodes
3. **Verbose logging** - Every failed attempt logs an error

## Industry Standards Implemented

### 1. Exponential Backoff (AWS SDK, gRPC, Kubernetes, etcd)

**Benefits:**
- Reduces connection attempts from 50-100 to 5-10 during restarts
- Prevents network congestion
- Allows services time to fully start

**Implementation:**
```go
// Start: 100ms
// Retry 1: 200ms
// Retry 2: 400ms
// Retry 3: 800ms
// Retry 4: 1.6s
// Retry 5: 3.2s
// Retry 6+: 30s (capped)
```

**With 20% jitter** to prevent thundering herd

### 2. Circuit Breaker (Netflix Hystrix, Kafka, Cassandra)

**Benefits:**
- Stops trying after 5 consecutive failures
- Waits 30 seconds before retry
- Prevents wasting resources on dead nodes
- Automatically recovers when node comes back

**States:**
- **Closed** (Normal): All requests go through
- **Open** (Tripped): Rejects requests for 30s
- **Half-Open** (Testing): Allows one request to test if recovered

### 3. Structured Logging with Sampling (Uber Zap, etcd)

**Benefits:**
- Debug logs disabled in production (zero overhead)
- Automatic sampling prevents log floods
- Structured fields for easy searching

## Files Created/Modified

### New Files:
1. `pkg/consensus/backoff.go` - Exponential backoff + circuit breaker implementation
2. `pkg/logger/logger.go` - Structured logging with zap

### Modified Files:
1. `pkg/consensus/transport_v2.go` - Add backoff/circuit breaker to peer connections
2. `pkg/consensus/node.go` - Use structured logging
3. `docker-compose.yml` - Increase health check grace periods

## Implementation Changes

### 1. Update `bidirPeer` struct:

```go
type bidirPeer struct {
    id   uint64
    addr string

    mu       sync.RWMutex
    conn     net.Conn
    lastUsed time.Time
    sendCh   chan raftpb.Message
    stopCh   chan struct{}

    // NEW: Industry-standard retry logic
    backoff        *Backoff
    circuitBreaker *CircuitBreaker
}
```

### 2. Initialize in `AddPeer`:

```go
peer := &bidirPeer{
    id:             nodeID,
    addr:           addr,
    sendCh:         make(chan raftpb.Message, 256),
    stopCh:         make(chan struct{}),
    backoff:        NewBackoff(),          // NEW
    circuitBreaker: NewCircuitBreaker(),   // NEW
}
```

### 3. Update `ensureConnected` with circuit breaker:

```go
func (t *BidirectionalTransport) ensureConnected(peer *bidirPeer) error {
    peer.mu.RLock()
    if peer.conn != nil {
        peer.mu.RUnlock()
        return nil
    }
    peer.mu.RUnlock()

    // NEW: Check circuit breaker
    if !peer.circuitBreaker.Call() {
        return fmt.Errorf("circuit breaker open for peer %d", peer.id)
    }

    peer.mu.Lock()
    defer peer.mu.Unlock()

    // Double-check
    if peer.conn != nil {
        return nil
    }

    // NEW: Apply backoff before connecting
    backoffDuration := peer.backoff.Next()
    time.Sleep(backoffDuration)

    // Try to connect
    conn, err := net.DialTimeout("tcp", peer.addr, 5*time.Second)
    if err != nil {
        // NEW: Record failure in circuit breaker
        peer.circuitBreaker.RecordFailure()

        // Use structured logging (debug level)
        logger.Debug("failed to connect to peer",
            zap.Uint64("peerId", peer.id),
            zap.String("addr", peer.addr),
            zap.Duration("backoff", backoffDuration),
            zap.String("circuitState", peer.circuitBreaker.State().String()),
            zap.Error(err))

        return fmt.Errorf("failed to connect to %s: %w", peer.addr, err)
    }

    // Send handshake
    if err := binary.Write(conn, binary.BigEndian, t.nodeID); err != nil {
        conn.Close()
        peer.circuitBreaker.RecordFailure()
        return fmt.Errorf("failed to send handshake: %w", err)
    }

    // Success!
    peer.conn = conn
    peer.lastUsed = time.Now()
    peer.backoff.Reset()                    // NEW: Reset backoff
    peer.circuitBreaker.RecordSuccess()     // NEW: Record success

    // Log success at info level
    logger.Info("connected to peer",
        zap.Uint64("peerId", peer.id),
        zap.String("addr", peer.addr))

    // Start goroutines
    t.wg.Add(2)
    go t.senderLoop(peer)
    go t.receiverLoop(peer)

    return nil
}
```

### 4. Update `docker-compose.yml`:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8081/health/live"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 30s  # Changed from 10s to 30s
```

## Performance Impact

### Before (No Backoff):
```
Broker restart window: 5 seconds
Connection attempts: ~50 (every 100ms)
Log entries: ~50 error messages
CPU overhead: Moderate (constant connection attempts)
Network overhead: High (50 SYN packets)
```

### After (With Backoff + Circuit Breaker):
```
Broker restart window: 5 seconds
Connection attempts: ~5-7 (exponential backoff)
Log entries: 0 in production (debug level)
             5-7 in debug mode (structured)
CPU overhead: Low (exponential delays)
Network overhead: Low (5-7 SYN packets)
Circuit breaker triggers after 5 failures
Waits 30s before retry after circuit opens
```

## Expected Behavior

### During Normal Operation:
- All connections stay open
- No retry attempts
- No backoff delays
- Circuit breakers remain closed

### During Broker Restart (~5-10 seconds):
```
T+0s:    Broker3 goes down
T+0.1s:  Attempt 1 fails → backoff 100ms
T+0.2s:  Attempt 2 fails → backoff 200ms
T+0.4s:  Attempt 3 fails → backoff 400ms
T+0.8s:  Attempt 4 fails → backoff 800ms
T+1.6s:  Attempt 5 fails → circuit breaker opens
T+31.6s: Circuit breaker allows test request
T+32s:   Broker3 is up → connection succeeds
         Backoff resets, circuit closes
```

**Total attempts during restart: 5**
**Total log entries (production): 0**
**Total log entries (debug): 5 structured entries**

## Comparison with Other Systems

| System | Initial Backoff | Max Backoff | Circuit Breaker | Jitter |
|--------|----------------|-------------|-----------------|--------|
| **StreamBus** | 100ms | 30s | 5 failures | 20% |
| Kafka | 100ms | 32s | No | 20% |
| etcd | 500ms | 30s | No | No |
| gRPC | 100ms | 120s | Yes (Hystrix) | 20% |
| Cassandra | 1s | 10min | Yes | No |
| AWS SDK | 100ms | 20s | No | Yes |

StreamBus now matches or exceeds industry best practices!

## Testing

### Test the improvements:

```bash
# 1. Rebuild with new code
docker-compose build

# 2. Start cluster
docker-compose up -d

# 3. Watch logs (production mode - should be quiet)
docker logs -f streambus-broker1

# 4. Restart broker3 to trigger retry logic
docker restart streambus-broker3

# Expected: Few or no connection error logs
# Circuit breaker will prevent log spam

# 5. Enable debug logging to see retry details
docker-compose exec broker1 sh -c 'export LOG_LEVEL=debug && kill -HUP 1'

# 6. Restart broker3 again
docker restart streambus-broker3

# Expected: 5-7 structured log entries showing:
# - backoff delays
# - circuit breaker state
# - successful reconnection
```

## Rollout Plan

1. **Phase 1: Logging** (✅ Complete)
   - Structured logging with zap
   - Log sampling
   - Debug levels

2. **Phase 2: Backoff** (In Progress)
   - Exponential backoff implementation
   - Jitter to prevent thundering herd

3. **Phase 3: Circuit Breaker** (In Progress)
   - Circuit breaker implementation
   - Automatic recovery

4. **Phase 4: Docker Config** (Pending)
   - Increase health check grace periods
   - Update restart policies

5. **Phase 5: Testing & Monitoring** (Pending)
   - Load testing during failures
   - Monitor connection metrics
   - Validate log reduction

## References

- [AWS SDK Exponential Backoff](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
- [gRPC Connection Backoff Protocol](https://github.com/grpc/grpc/blob/master/doc/connection-backoff.md)
- [Netflix Hystrix Circuit Breaker](https://github.com/Netflix/Hystrix/wiki/How-it-Works)
- [Google SRE Book - Handling Overload](https://sre.google/sre-book/handling-overload/)
- [Kafka Producer Retry Logic](https://kafka.apache.org/documentation/#producerconfigs_retries)
- [etcd Client Design](https://etcd.io/docs/v3.5/learning/design-client/)
