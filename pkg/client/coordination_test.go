package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/server"
)

// fakeCoordinationServer is a minimal RequestHandler that answers
// FindCoordinator and Heartbeat with canned, swappable responses and counts
// how many times each was called. It lets the caching, invalidation and
// old-server fallback behaviour in coordination.go be tested without
// standing up a real group coordinator.
type fakeCoordinationServer struct {
	mu sync.Mutex

	findCoordinatorCalls int
	findCoordinatorResp  *protocol.FindCoordinatorResponse
	// findCoordinatorUnknown makes FindCoordinator answer the way a broker
	// that predates this request type would: an unrecognised-request error,
	// exactly what server/handler.go's default case returns today.
	findCoordinatorUnknown bool

	heartbeatCalls int
	heartbeatResp  *protocol.HeartbeatResponse
}

func (h *fakeCoordinationServer) Handle(req *protocol.Request) *protocol.Response {
	switch req.Header.Type {
	case protocol.RequestTypeFindCoordinator:
		h.mu.Lock()
		h.findCoordinatorCalls++
		unknown := h.findCoordinatorUnknown
		resp := h.findCoordinatorResp
		h.mu.Unlock()

		if unknown {
			return &protocol.Response{
				Header: protocol.ResponseHeader{
					RequestID: req.Header.RequestID,
					Status:    protocol.StatusError,
					ErrorCode: protocol.ErrUnknownRequest,
				},
				Payload: &protocol.ErrorResponse{ErrorCode: protocol.ErrUnknownRequest, Message: "unknown request type"},
			}
		}
		return &protocol.Response{
			Header:  protocol.ResponseHeader{RequestID: req.Header.RequestID, Status: protocol.StatusOK},
			Payload: resp,
		}

	case protocol.RequestTypeHeartbeat:
		h.mu.Lock()
		h.heartbeatCalls++
		resp := h.heartbeatResp
		h.mu.Unlock()
		return &protocol.Response{
			Header:  protocol.ResponseHeader{RequestID: req.Header.RequestID, Status: protocol.StatusOK},
			Payload: resp,
		}

	default:
		return &protocol.Response{
			Header: protocol.ResponseHeader{
				RequestID: req.Header.RequestID,
				Status:    protocol.StatusError,
				ErrorCode: protocol.ErrUnknownRequest,
			},
			Payload: &protocol.ErrorResponse{ErrorCode: protocol.ErrUnknownRequest, Message: "unknown request type"},
		}
	}
}

func (h *fakeCoordinationServer) counts() (findCoordinator, heartbeat int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.findCoordinatorCalls, h.heartbeatCalls
}

// startFakeCoordinationServer starts handler on an ephemeral port and stops
// it when the test ends, returning the address to dial.
func startFakeCoordinationServer(t *testing.T, handler server.RequestHandler) string {
	t.Helper()

	config := server.DefaultConfig()
	config.Address = "127.0.0.1:0"

	srv, err := server.New(config, handler)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("srv.Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	return srv.Listener().Addr().String()
}

// newFastRetryClient builds a client with retries effectively disabled, so a
// test that deliberately makes a request fail (the old-server fallback
// case) does not pay sendRequestWithRetry's backoff.
func newFastRetryClient(t *testing.T, addr string) *Client {
	t.Helper()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.RequestTimeout = 5 * time.Second
	config.MaxRetries = 0

	c, err := New(config)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func mustSplitHostPort(t *testing.T, addr string) (string, int32) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parsing port %q: %v", portStr, err)
	}
	return host, int32(port)
}

func TestSendCoordinationRequest_CachesCoordinator(t *testing.T) {
	fake := &fakeCoordinationServer{heartbeatResp: &protocol.HeartbeatResponse{ErrorCode: protocol.ErrNone}}
	addr := startFakeCoordinationServer(t, fake)
	host, port := mustSplitHostPort(t, addr)
	fake.findCoordinatorResp = &protocol.FindCoordinatorResponse{NodeID: 1, Host: host, Port: port}

	c := newFastRetryClient(t, addr)
	ctx := context.Background()

	if _, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "analytics"}); err != nil {
		t.Fatalf("first Heartbeat: %v", err)
	}
	if _, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "analytics"}); err != nil {
		t.Fatalf("second Heartbeat: %v", err)
	}

	findCoordinator, heartbeat := fake.counts()
	if findCoordinator != 1 {
		t.Errorf("FindCoordinator called %d times, want 1 (second Heartbeat should hit the cache)", findCoordinator)
	}
	if heartbeat != 2 {
		t.Errorf("Heartbeat called %d times, want 2", heartbeat)
	}
}

func TestSendCoordinationRequest_CachesSeparatelyPerGroup(t *testing.T) {
	fake := &fakeCoordinationServer{heartbeatResp: &protocol.HeartbeatResponse{ErrorCode: protocol.ErrNone}}
	addr := startFakeCoordinationServer(t, fake)
	host, port := mustSplitHostPort(t, addr)
	fake.findCoordinatorResp = &protocol.FindCoordinatorResponse{NodeID: 1, Host: host, Port: port}

	c := newFastRetryClient(t, addr)
	ctx := context.Background()

	if _, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "analytics"}); err != nil {
		t.Fatalf("Heartbeat(analytics): %v", err)
	}
	if _, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "billing"}); err != nil {
		t.Fatalf("Heartbeat(billing): %v", err)
	}

	findCoordinator, _ := fake.counts()
	if findCoordinator != 2 {
		t.Errorf("FindCoordinator called %d times, want 2 (one per distinct group ID)", findCoordinator)
	}
}

func TestSendCoordinationRequest_InvalidatesOnNotCoordinator(t *testing.T) {
	fake := &fakeCoordinationServer{heartbeatResp: &protocol.HeartbeatResponse{ErrorCode: protocol.ErrNotCoordinator}}
	addr := startFakeCoordinationServer(t, fake)
	host, port := mustSplitHostPort(t, addr)
	fake.findCoordinatorResp = &protocol.FindCoordinatorResponse{NodeID: 1, Host: host, Port: port}

	c := newFastRetryClient(t, addr)
	ctx := context.Background()

	// The first Heartbeat resolves the coordinator, then the (fake) broker
	// reports it is not the coordinator after all.
	resp, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "analytics"})
	if err != nil {
		t.Fatalf("first Heartbeat: %v", err)
	}
	if resp.ErrorCode != protocol.ErrNotCoordinator {
		t.Fatalf("first Heartbeat error = %v, want ErrNotCoordinator", resp.ErrorCode)
	}

	// Flip the canned response so the second call can succeed and prove
	// resolution actually happened again rather than reusing a bad cache
	// entry forever.
	fake.mu.Lock()
	fake.heartbeatResp = &protocol.HeartbeatResponse{ErrorCode: protocol.ErrNone}
	fake.mu.Unlock()

	if _, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "analytics"}); err != nil {
		t.Fatalf("second Heartbeat: %v", err)
	}

	findCoordinator, heartbeat := fake.counts()
	if findCoordinator != 2 {
		t.Errorf("FindCoordinator called %d times, want 2 (the stale cache entry must be re-resolved)", findCoordinator)
	}
	if heartbeat != 2 {
		t.Errorf("Heartbeat called %d times, want 2", heartbeat)
	}
}

func TestSendCoordinationRequest_FallsBackWhenServerPredatesFindCoordinator(t *testing.T) {
	// A broker that does not recognise RequestTypeFindCoordinator answers
	// with ErrUnknownRequest, exactly as server/handler.go's default case
	// does for any type it does not know. The client must still be able to
	// reach that broker for requests it does understand.
	fake := &fakeCoordinationServer{
		findCoordinatorUnknown: true,
		heartbeatResp:          &protocol.HeartbeatResponse{ErrorCode: protocol.ErrNone},
	}
	addr := startFakeCoordinationServer(t, fake)

	c := newFastRetryClient(t, addr)
	ctx := context.Background()

	resp, err := c.Heartbeat(ctx, &protocol.HeartbeatRequest{GroupID: "analytics"})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if resp.ErrorCode != protocol.ErrNone {
		t.Errorf("Heartbeat error = %v, want ErrNone", resp.ErrorCode)
	}

	findCoordinator, heartbeat := fake.counts()
	if findCoordinator == 0 {
		t.Error("FindCoordinator should have been tried at least once before falling back")
	}
	if heartbeat != 1 {
		t.Errorf("Heartbeat called %d times, want 1 (fallback to the configured broker must still work)", heartbeat)
	}

	// The failed resolution must not be cached as if it had succeeded: a
	// second call should still be willing to try FindCoordinator again
	// rather than being stuck on a cached failure.
	if _, ok := c.coordCache.get(protocol.CoordinatorKeyTypeGroup, "analytics"); ok {
		t.Error("a failed FindCoordinator resolution must not populate the cache")
	}
}

func TestCoordinatorCache_GroupAndTransactionKeysDoNotCollide(t *testing.T) {
	cache := newCoordinatorCache()
	cache.set(protocol.CoordinatorKeyTypeGroup, "shared-name", "broker-a:9092")
	cache.set(protocol.CoordinatorKeyTypeTransaction, "shared-name", "broker-b:9092")

	group, ok := cache.get(protocol.CoordinatorKeyTypeGroup, "shared-name")
	if !ok || group != "broker-a:9092" {
		t.Errorf("group entry = (%q, %v), want (broker-a:9092, true)", group, ok)
	}
	txn, ok := cache.get(protocol.CoordinatorKeyTypeTransaction, "shared-name")
	if !ok || txn != "broker-b:9092" {
		t.Errorf("transaction entry = (%q, %v), want (broker-b:9092, true)", txn, ok)
	}

	cache.invalidate(protocol.CoordinatorKeyTypeGroup, "shared-name")
	if _, ok := cache.get(protocol.CoordinatorKeyTypeGroup, "shared-name"); ok {
		t.Error("group entry should have been invalidated")
	}
	if _, ok := cache.get(protocol.CoordinatorKeyTypeTransaction, "shared-name"); !ok {
		t.Error("invalidating the group entry must not touch the transaction entry")
	}
}

func TestCoordinationKeyFor(t *testing.T) {
	tests := []struct {
		name        string
		payload     interface{}
		wantKeyType protocol.CoordinatorKeyType
		wantKey     string
		wantOK      bool
	}{
		{"JoinGroup", &protocol.JoinGroupRequest{GroupID: "g"}, protocol.CoordinatorKeyTypeGroup, "g", true},
		{"SyncGroup", &protocol.SyncGroupRequest{GroupID: "g"}, protocol.CoordinatorKeyTypeGroup, "g", true},
		{"Heartbeat", &protocol.HeartbeatRequest{GroupID: "g"}, protocol.CoordinatorKeyTypeGroup, "g", true},
		{"LeaveGroup", &protocol.LeaveGroupRequest{GroupID: "g"}, protocol.CoordinatorKeyTypeGroup, "g", true},
		{"OffsetCommit", &protocol.OffsetCommitRequest{GroupID: "g"}, protocol.CoordinatorKeyTypeGroup, "g", true},
		{"OffsetFetch", &protocol.OffsetFetchRequest{GroupID: "g"}, protocol.CoordinatorKeyTypeGroup, "g", true},
		{"InitProducerID", &protocol.InitProducerIDRequest{TransactionID: "t"}, protocol.CoordinatorKeyTypeTransaction, "t", true},
		{"AddPartitionsToTxn", &protocol.AddPartitionsToTxnRequest{TransactionID: "t"}, protocol.CoordinatorKeyTypeTransaction, "t", true},
		{"AddOffsetsToTxn", &protocol.AddOffsetsToTxnRequest{TransactionID: "t"}, protocol.CoordinatorKeyTypeTransaction, "t", true},
		{"TxnOffsetCommit", &protocol.TxnOffsetCommitRequest{TransactionID: "t"}, protocol.CoordinatorKeyTypeTransaction, "t", true},
		{"EndTxn", &protocol.EndTxnRequest{TransactionID: "t"}, protocol.CoordinatorKeyTypeTransaction, "t", true},
		{"ListTopics has no key", &protocol.ListTopicsRequest{}, 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyType, key, ok := coordinationKeyFor(tt.payload)
			if ok != tt.wantOK || keyType != tt.wantKeyType || key != tt.wantKey {
				t.Errorf("coordinationKeyFor(%T) = (%v, %q, %v), want (%v, %q, %v)",
					tt.payload, keyType, key, ok, tt.wantKeyType, tt.wantKey, tt.wantOK)
			}
		})
	}
}

func TestResponseErrorCodeAndIsNotCoordinatorError(t *testing.T) {
	tests := []struct {
		name         string
		payload      interface{}
		wantCode     protocol.ErrorCode
		wantStaleErr bool
	}{
		{"JoinGroup none", &protocol.JoinGroupResponse{}, protocol.ErrNone, false},
		{"Heartbeat not coordinator", &protocol.HeartbeatResponse{ErrorCode: protocol.ErrNotCoordinator}, protocol.ErrNotCoordinator, true},
		{"InitProducerID coordinator unavailable", &protocol.InitProducerIDResponse{ErrorCode: protocol.ErrTransactionCoordinatorNotAvailable}, protocol.ErrTransactionCoordinatorNotAvailable, true},
		{"EndTxn fenced", &protocol.EndTxnResponse{ErrorCode: protocol.ErrTransactionCoordinatorFenced}, protocol.ErrTransactionCoordinatorFenced, true},
		{"EndTxn unrelated error", &protocol.EndTxnResponse{ErrorCode: protocol.ErrInvalidTransactionState}, protocol.ErrInvalidTransactionState, false},
		{
			"OffsetCommit partition failure via FirstError",
			&protocol.OffsetCommitResponse{Topics: []protocol.OffsetCommitTopicResult{
				{Topic: "orders", Partitions: []protocol.OffsetCommitPartitionResult{{Partition: 0, ErrorCode: protocol.ErrNotCoordinator}}},
			}},
			protocol.ErrNotCoordinator, true,
		},
		{"unrecognised payload", &protocol.ListTopicsResponse{}, protocol.ErrNone, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := responseErrorCode(tt.payload)
			if code != tt.wantCode {
				t.Errorf("responseErrorCode(%T) = %v, want %v", tt.payload, code, tt.wantCode)
			}
			if isNotCoordinatorError(code) != tt.wantStaleErr {
				t.Errorf("isNotCoordinatorError(%v) = %v, want %v", code, !tt.wantStaleErr, tt.wantStaleErr)
			}
		})
	}
}
