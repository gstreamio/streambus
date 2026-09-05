package client

import (
	"context"
	"fmt"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// Consumer group and transaction RPCs.
//
// Every call goes to the coordinator broker. StreamBus does not yet expose a
// FindCoordinator request, so the first configured broker acts as coordinator
// for every group and transactional ID; when FindCoordinator arrives, only
// coordinatorBroker below needs to change.

// coordinatorBroker returns the broker to send coordination requests to.
func (c *Client) coordinatorBroker() (string, error) {
	if len(c.config.Brokers) == 0 {
		return "", fmt.Errorf("no brokers configured")
	}
	return c.config.Brokers[0], nil
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

	broker, err := c.coordinatorBroker()
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

	return resp.Payload, nil
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

	broker, err := c.coordinatorBroker()
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
