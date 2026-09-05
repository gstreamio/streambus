package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// Consumer group and transaction RPCs.
//
// Every call goes to the coordinator broker for its group ID or
// transactional ID. The coordinator is discovered with FindCoordinator and
// cached per key on the Client (coordCache), so a hot path like heartbeats
// or offset commits does not re-resolve on every call. A response reporting
// that the broker it hit is no longer (or never was) the coordinator
// invalidates that key's cache entry, so the next call re-resolves instead
// of retrying the same wrong broker forever.

// coordinatorCacheKey identifies one coordination key: a group ID resolved
// as CoordinatorKeyTypeGroup, or a transactional ID resolved as
// CoordinatorKeyTypeTransaction. The two spaces are kept separate because
// nothing stops a group ID and a transactional ID from being equal strings.
type coordinatorCacheKey struct {
	keyType protocol.CoordinatorKeyType
	key     string
}

// coordinatorCache maps a coordination key to the broker address last named
// as its coordinator.
type coordinatorCache struct {
	mu      sync.RWMutex
	entries map[coordinatorCacheKey]string
}

func newCoordinatorCache() *coordinatorCache {
	return &coordinatorCache{entries: make(map[coordinatorCacheKey]string)}
}

func (c *coordinatorCache) get(keyType protocol.CoordinatorKeyType, key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	broker, ok := c.entries[coordinatorCacheKey{keyType, key}]
	return broker, ok
}

func (c *coordinatorCache) set(keyType protocol.CoordinatorKeyType, key, broker string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[coordinatorCacheKey{keyType, key}] = broker
}

func (c *coordinatorCache) invalidate(keyType protocol.CoordinatorKeyType, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, coordinatorCacheKey{keyType, key})
}

// coordinationKeyFor extracts the group ID or transactional ID that
// identifies which coordinator a request payload must reach. Payloads with
// no such key (there are none among the coordination RPCs today, but
// sendCoordinationRequest must still handle the case) report ok=false, and
// the caller falls back to the first configured broker rather than failing.
func coordinationKeyFor(payload interface{}) (keyType protocol.CoordinatorKeyType, key string, ok bool) {
	switch p := payload.(type) {
	case *protocol.JoinGroupRequest:
		return protocol.CoordinatorKeyTypeGroup, p.GroupID, true
	case *protocol.SyncGroupRequest:
		return protocol.CoordinatorKeyTypeGroup, p.GroupID, true
	case *protocol.HeartbeatRequest:
		return protocol.CoordinatorKeyTypeGroup, p.GroupID, true
	case *protocol.LeaveGroupRequest:
		return protocol.CoordinatorKeyTypeGroup, p.GroupID, true
	case *protocol.OffsetCommitRequest:
		return protocol.CoordinatorKeyTypeGroup, p.GroupID, true
	case *protocol.OffsetFetchRequest:
		return protocol.CoordinatorKeyTypeGroup, p.GroupID, true
	case *protocol.InitProducerIDRequest:
		return protocol.CoordinatorKeyTypeTransaction, p.TransactionID, true
	case *protocol.AddPartitionsToTxnRequest:
		return protocol.CoordinatorKeyTypeTransaction, p.TransactionID, true
	case *protocol.AddOffsetsToTxnRequest:
		return protocol.CoordinatorKeyTypeTransaction, p.TransactionID, true
	case *protocol.TxnOffsetCommitRequest:
		return protocol.CoordinatorKeyTypeTransaction, p.TransactionID, true
	case *protocol.EndTxnRequest:
		return protocol.CoordinatorKeyTypeTransaction, p.TransactionID, true
	default:
		return 0, "", false
	}
}

// responseErrorCode extracts the primary error code from a coordination
// response payload, used only to detect a stale cache entry (see
// isNotCoordinatorError). Payloads that report per-member or per-partition
// results rather than one code use their own FirstError, mirroring how
// callers already inspect them; a code of ErrNone here never suppresses a
// real per-item failure, it only means "nothing here looked like a
// wrong-coordinator response".
func responseErrorCode(payload interface{}) protocol.ErrorCode {
	switch p := payload.(type) {
	case *protocol.JoinGroupResponse:
		return p.ErrorCode
	case *protocol.SyncGroupResponse:
		return p.ErrorCode
	case *protocol.HeartbeatResponse:
		return p.ErrorCode
	case *protocol.LeaveGroupResponse:
		return p.ErrorCode
	case *protocol.OffsetCommitResponse:
		return p.FirstError()
	case *protocol.InitProducerIDResponse:
		return p.ErrorCode
	case *protocol.AddPartitionsToTxnResponse:
		return p.FirstError()
	case *protocol.AddOffsetsToTxnResponse:
		return p.ErrorCode
	case *protocol.TxnOffsetCommitResponse:
		return p.FirstError()
	case *protocol.EndTxnResponse:
		return p.ErrorCode
	default:
		return protocol.ErrNone
	}
}

// isNotCoordinatorError reports whether code means the broker that answered
// is not the right one for this key, so the caller's cache entry is stale.
func isNotCoordinatorError(code protocol.ErrorCode) bool {
	switch code {
	case protocol.ErrNotCoordinator,
		protocol.ErrTransactionCoordinatorNotAvailable,
		protocol.ErrTransactionCoordinatorFenced:
		return true
	default:
		return false
	}
}

// coordinatorBroker returns the broker to send a coordination request for
// (keyType, key) to, resolving and caching it via FindCoordinator on a
// cache miss.
//
// Concurrent callers that miss the cache for the same key are not
// coordinated with each other: each runs its own resolveCoordinator and the
// last one to finish wins the cache entry (they should all agree anyway,
// since FindCoordinator is deterministic for a stable broker set). This is
// deliberate, not an oversight - the duplication is bounded by however many
// goroutines happen to race the same miss, and self-limiting, since every
// subsequent call for that key hits the cache once any winner has populated
// it. Single-flighting would remove the duplicate round trips, but isn't
// worth the extra bookkeeping unless resolution becomes expensive or
// rate-limited, neither of which is true of a single small request today.
func (c *Client) coordinatorBroker(ctx context.Context, keyType protocol.CoordinatorKeyType, key string) (string, error) {
	if len(c.config.Brokers) == 0 {
		return "", fmt.Errorf("no brokers configured")
	}

	if broker, ok := c.coordCache.get(keyType, key); ok {
		return broker, nil
	}

	if broker, ok := c.resolveCoordinator(ctx, keyType, key); ok {
		c.coordCache.set(keyType, key, broker)
		return broker, nil
	}

	// FindCoordinator could not be resolved against any configured broker -
	// fall back to the first one without caching the guess, so the next
	// call tries real resolution again instead of being stuck on a fallback
	// that was never confirmed correct.
	return c.config.Brokers[0], nil
}

// resolveCoordinator asks each configured broker, in turn, which broker
// coordinates (keyType, key). ok is false if none could answer.
//
// Two situations look identical from here and are handled the same way: a
// broker running a version too old to know RequestTypeFindCoordinator
// answers with ErrUnknownRequest, which sendRequestWithRetry turns into a
// plain error just like a connection failure would. Either way, resolution
// keeps trying the remaining configured brokers before giving up - a
// rolling upgrade with an old broker still in the mix should not have to
// depend on which broker happens to be first in the list.
//
// Miss latency scales with how many unreachable brokers sit ahead of a live
// one in Brokers, since each is tried (and its own retries exhausted) in
// order before the next is attempted; this is the accepted cost of not
// needing any out-of-band knowledge of which brokers are actually up.
func (c *Client) resolveCoordinator(ctx context.Context, keyType protocol.CoordinatorKeyType, key string) (string, bool) {
	for _, broker := range c.config.Brokers {
		resp, err := c.sendRequestWithRetry(ctx, broker, &protocol.Request{
			Header: protocol.RequestHeader{
				Type:    protocol.RequestTypeFindCoordinator,
				Version: protocol.ProtocolVersion,
				Flags:   protocol.FlagNone,
			},
			Payload: &protocol.FindCoordinatorRequest{Key: key, KeyType: keyType},
		})
		if err != nil {
			continue
		}
		found, ok := resp.Payload.(*protocol.FindCoordinatorResponse)
		if !ok || found.ErrorCode != protocol.ErrNone {
			continue
		}
		return fmt.Sprintf("%s:%d", found.Host, found.Port), true
	}

	return "", false
}

// sendCoordinationRequest sends a coordination request to the coordinator and
// returns the decoded response payload.
func (c *Client) sendCoordinationRequest(
	ctx context.Context,
	reqType protocol.RequestType,
	payload interface{},
) (interface{}, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, ErrClientClosed
	}

	keyType, key, hasKey := coordinationKeyFor(payload)

	var broker string
	var err error
	if hasKey {
		broker, err = c.coordinatorBroker(ctx, keyType, key)
	} else {
		broker, err = c.firstConfiguredBroker()
	}
	if err != nil {
		return nil, err
	}

	resp, err := c.sendRequestWithRetry(ctx, broker, &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    reqType,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}

	// A stale cache entry must not keep pointing a group or transaction at a
	// broker that just said it is not the coordinator.
	if hasKey && isNotCoordinatorError(responseErrorCode(resp.Payload)) {
		c.coordCache.invalidate(keyType, key)
	}

	return resp.Payload, nil
}

// firstConfiguredBroker returns the first configured broker, for requests
// (like ListTopics) that are not scoped to any group or transactional ID and
// so have no coordinator to resolve.
func (c *Client) firstConfiguredBroker() (string, error) {
	if len(c.config.Brokers) == 0 {
		return "", fmt.Errorf("no brokers configured")
	}
	return c.config.Brokers[0], nil
}

// JoinGroup asks the coordinator to admit this member to a consumer group.
func (c *Client) JoinGroup(ctx context.Context, req *protocol.JoinGroupRequest) (*protocol.JoinGroupResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeJoinGroup, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.JoinGroupResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// SyncGroup exchanges the leader's assignment for this member's share of it.
func (c *Client) SyncGroup(ctx context.Context, req *protocol.SyncGroupRequest) (*protocol.SyncGroupResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeSyncGroup, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.SyncGroupResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// Heartbeat keeps this member's group session alive.
func (c *Client) Heartbeat(ctx context.Context, req *protocol.HeartbeatRequest) (*protocol.HeartbeatResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeHeartbeat, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.HeartbeatResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// LeaveGroup removes this member from its group.
func (c *Client) LeaveGroup(ctx context.Context, req *protocol.LeaveGroupRequest) (*protocol.LeaveGroupResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeLeaveGroup, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.LeaveGroupResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// CommitOffsets stores consumer positions in the group.
func (c *Client) CommitOffsets(ctx context.Context, req *protocol.OffsetCommitRequest) (*protocol.OffsetCommitResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeOffsetCommit, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.OffsetCommitResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// FetchOffsets retrieves committed positions for a group.
func (c *Client) FetchOffsets(ctx context.Context, req *protocol.OffsetFetchRequest) (*protocol.OffsetFetchResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeOffsetFetch, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.OffsetFetchResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// InitProducerID claims a producer ID for a transactional ID.
func (c *Client) InitProducerID(ctx context.Context, req *protocol.InitProducerIDRequest) (*protocol.InitProducerIDResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeInitProducerID, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.InitProducerIDResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// AddPartitionsToTxn registers partitions with an open transaction.
func (c *Client) AddPartitionsToTxn(ctx context.Context, req *protocol.AddPartitionsToTxnRequest) (*protocol.AddPartitionsToTxnResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeAddPartitionsToTxn, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.AddPartitionsToTxnResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// AddOffsetsToTxn brings a consumer group's offsets into a transaction.
func (c *Client) AddOffsetsToTxn(ctx context.Context, req *protocol.AddOffsetsToTxnRequest) (*protocol.AddOffsetsToTxnResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeAddOffsetsToTxn, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.AddOffsetsToTxnResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// TxnOffsetCommit commits consumer offsets inside a transaction.
func (c *Client) TxnOffsetCommit(ctx context.Context, req *protocol.TxnOffsetCommitRequest) (*protocol.TxnOffsetCommitResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeTxnOffsetCommit, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.TxnOffsetCommitResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// EndTxn commits or aborts a transaction.
func (c *Client) EndTxn(ctx context.Context, req *protocol.EndTxnRequest) (*protocol.EndTxnResponse, error) {
	payload, err := c.sendCoordinationRequest(ctx, protocol.RequestTypeEndTxn, req)
	if err != nil {
		return nil, err
	}
	resp, ok := payload.(*protocol.EndTxnResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}
	return resp, nil
}

// TopicPartitionCounts returns the partition count for each named topic,
// used by a group leader to know what there is to assign.
func (c *Client) TopicPartitionCounts(ctx context.Context, topics []string) (map[string]uint32, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, ErrClientClosed
	}

	broker, err := c.firstConfiguredBroker()
	if err != nil {
		return nil, err
	}

	resp, err := c.sendRequestWithRetry(ctx, broker, &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeListTopics,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.ListTopicsRequest{},
	})
	if err != nil {
		return nil, err
	}

	listResp, ok := resp.Payload.(*protocol.ListTopicsResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}

	wanted := make(map[string]bool, len(topics))
	for _, topic := range topics {
		wanted[topic] = true
	}

	// Topics the broker does not know about are simply absent from the
	// result; the caller decides whether that is an error. Reporting them as
	// zero-partition topics would silently produce an empty assignment.
	counts := make(map[string]uint32, len(topics))
	for _, info := range listResp.Topics {
		if wanted[info.Name] {
			counts[info.Name] = info.NumPartitions
		}
	}

	return counts, nil
}

// coordinationErrorText renders a coordination error code for an error message.
func coordinationErrorText(code protocol.ErrorCode) string {
	return fmt.Sprintf("%s (code %d)", code.String(), uint16(code))
}
