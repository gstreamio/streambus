package server

import (
	"sort"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// CoordinationHandler routes consumer group requests to a group coordinator
// and delegates everything else to the handler it wraps.
//
// It is the bridge that was missing between the wire protocol and the
// already-implemented group.GroupCoordinator: without it, JoinGroup and
// friends arrive on the socket and fall through to "unknown request type".
type CoordinationHandler struct {
	baseHandler RequestHandler
	coordinator *group.GroupCoordinator
}

// NewCoordinationHandler wraps baseHandler so consumer group requests are
// served by coordinator. A nil coordinator makes group requests fail with
// ErrNotCoordinator rather than being silently ignored.
func NewCoordinationHandler(baseHandler RequestHandler, coordinator *group.GroupCoordinator) *CoordinationHandler {
	return &CoordinationHandler{
		baseHandler: baseHandler,
		coordinator: coordinator,
	}
}

// Handle routes a request.
func (h *CoordinationHandler) Handle(req *protocol.Request) *protocol.Response {
	switch req.Header.Type {
	case protocol.RequestTypeJoinGroup:
		return h.handleJoinGroup(req)
	case protocol.RequestTypeSyncGroup:
		return h.handleSyncGroup(req)
	case protocol.RequestTypeHeartbeat:
		return h.handleHeartbeat(req)
	case protocol.RequestTypeLeaveGroup:
		return h.handleLeaveGroup(req)
	case protocol.RequestTypeOffsetCommit:
		return h.handleOffsetCommit(req)
	case protocol.RequestTypeOffsetFetch:
		return h.handleOffsetFetch(req)
	default:
		return h.baseHandler.Handle(req)
	}
}

// available reports whether a coordinator is wired up, returning an error
// response if not.
func (h *CoordinationHandler) available(req *protocol.Request) *protocol.Response {
	if h.coordinator != nil {
		return nil
	}
	return coordinationError(req.Header.RequestID, protocol.ErrNotCoordinator,
		"this broker is not a consumer group coordinator")
}

// okResponse builds a successful response carrying a coordination payload.
func okResponse(requestID uint64, payload interface{}) *protocol.Response {
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusOK,
		},
		Payload: payload,
	}
}

// coordinationError builds a transport-level error response, used when the
// request could not be processed at all. Per-member and per-partition
// failures travel in the payload's error codes instead.
func coordinationError(requestID uint64, code protocol.ErrorCode, message string) *protocol.Response {
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: code,
		},
		Payload: &protocol.ErrorResponse{ErrorCode: code, Message: message},
	}
}

// badRequest builds the response for a payload that does not match its
// request type. This means a protocol violation, not a coordination failure.
func badRequest(requestID uint64, expected string) *protocol.Response {
	return coordinationError(requestID, protocol.ErrInvalidRequest,
		"payload does not match request type: expected "+expected)
}

func (h *CoordinationHandler) handleJoinGroup(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.JoinGroupRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "JoinGroupRequest")
	}

	protocols := make([]group.ProtocolMetadata, 0, len(payload.Protocols))
	for _, p := range payload.Protocols {
		protocols = append(protocols, group.ProtocolMetadata{Name: p.Name, Metadata: p.Metadata})
	}

	result, err := h.coordinator.HandleJoinGroup(&group.JoinGroupRequest{
		GroupID:            payload.GroupID,
		SessionTimeoutMs:   payload.SessionTimeoutMs,
		RebalanceTimeoutMs: payload.RebalanceTimeoutMs,
		MemberID:           payload.MemberID,
		ClientID:           payload.ClientID,
		ProtocolType:       payload.ProtocolType,
		Protocols:          protocols,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	members := make([]protocol.JoinGroupMember, 0, len(result.Members))
	for _, m := range result.Members {
		members = append(members, protocol.JoinGroupMember{MemberID: m.MemberID, Metadata: m.Metadata})
	}

	return okResponse(req.Header.RequestID, &protocol.JoinGroupResponse{
		ErrorCode:    groupErrorToProtocol(result.ErrorCode),
		GenerationID: result.GenerationID,
		ProtocolName: result.ProtocolName,
		MemberID:     result.MemberID,
		LeaderID:     result.LeaderID,
		Members:      members,
	})
}

func (h *CoordinationHandler) handleSyncGroup(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.SyncGroupRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "SyncGroupRequest")
	}

	assignments := make([]group.MemberAssignmentData, 0, len(payload.Assignments))
	for _, a := range payload.Assignments {
		assignments = append(assignments, group.MemberAssignmentData{
			MemberID:   a.MemberID,
			Assignment: a.Assignment,
		})
	}

	result, err := h.coordinator.HandleSyncGroup(&group.SyncGroupRequest{
		GroupID:      payload.GroupID,
		GenerationID: payload.GenerationID,
		MemberID:     payload.MemberID,
		Assignments:  assignments,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.SyncGroupResponse{
		ErrorCode:  groupErrorToProtocol(result.ErrorCode),
		Assignment: result.Assignment,
	})
}

func (h *CoordinationHandler) handleHeartbeat(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.HeartbeatRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "HeartbeatRequest")
	}

	result, err := h.coordinator.HandleHeartbeat(&group.HeartbeatRequest{
		GroupID:      payload.GroupID,
		GenerationID: payload.GenerationID,
		MemberID:     payload.MemberID,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.HeartbeatResponse{
		ErrorCode: groupErrorToProtocol(result.ErrorCode),
	})
}

func (h *CoordinationHandler) handleLeaveGroup(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.LeaveGroupRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "LeaveGroupRequest")
	}

	result, err := h.coordinator.HandleLeaveGroup(&group.LeaveGroupRequest{
		GroupID:  payload.GroupID,
		MemberID: payload.MemberID,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.LeaveGroupResponse{
		ErrorCode: groupErrorToProtocol(result.ErrorCode),
	})
}

func (h *CoordinationHandler) handleOffsetCommit(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.OffsetCommitRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "OffsetCommitRequest")
	}

	offsets := make(map[string]map[int32]group.OffsetCommitData, len(payload.Topics))
	for _, topic := range payload.Topics {
		partitions := make(map[int32]group.OffsetCommitData, len(topic.Partitions))
		for _, p := range topic.Partitions {
			partitions[p.Partition] = group.OffsetCommitData{Offset: p.Offset, Metadata: p.Metadata}
		}
		offsets[topic.Topic] = partitions
	}

	result, err := h.coordinator.HandleOffsetCommit(&group.OffsetCommitRequest{
		GroupID:      payload.GroupID,
		GenerationID: payload.GenerationID,
		MemberID:     payload.MemberID,
		Offsets:      offsets,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	// Report results in the order the request listed them, so a client can
	// line responses up with what it sent without sorting.
	topics := make([]protocol.OffsetCommitTopicResult, 0, len(payload.Topics))
	for _, topic := range payload.Topics {
		results := make([]protocol.OffsetCommitPartitionResult, 0, len(topic.Partitions))
		for _, p := range topic.Partitions {
			results = append(results, protocol.OffsetCommitPartitionResult{
				Partition: p.Partition,
				ErrorCode: groupErrorToProtocol(result.Errors[topic.Topic][p.Partition]),
			})
		}
		topics = append(topics, protocol.OffsetCommitTopicResult{Topic: topic.Topic, Partitions: results})
	}

	return okResponse(req.Header.RequestID, &protocol.OffsetCommitResponse{Topics: topics})
}

func (h *CoordinationHandler) handleOffsetFetch(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.OffsetFetchRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "OffsetFetchRequest")
	}

	var topics map[string][]int32
	if len(payload.Topics) > 0 {
		topics = make(map[string][]int32, len(payload.Topics))
		for _, topic := range payload.Topics {
			topics[topic.Topic] = topic.Partitions
		}
	}

	result, err := h.coordinator.HandleOffsetFetch(&group.OffsetFetchRequest{
		GroupID: payload.GroupID,
		Topics:  topics,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.OffsetFetchResponse{
		Topics: buildOffsetFetchTopics(payload, result),
	})
}

// buildOffsetFetchTopics converts coordinator offsets into wire form. When the
// request named topics, the response follows that order; when it asked for
// everything, topics are sorted so the response is deterministic.
func buildOffsetFetchTopics(
	payload *protocol.OffsetFetchRequest,
	result *group.OffsetFetchResponse,
) []protocol.OffsetFetchTopicResult {
	if len(payload.Topics) > 0 {
		topics := make([]protocol.OffsetFetchTopicResult, 0, len(payload.Topics))
		for _, requested := range payload.Topics {
			fetched := result.Offsets[requested.Topic]
			partitions := make([]protocol.OffsetFetchPartition, 0, len(requested.Partitions))
			for _, partition := range requested.Partitions {
				partitions = append(partitions, toOffsetFetchPartition(partition, fetched[partition]))
			}
			topics = append(topics, protocol.OffsetFetchTopicResult{
				Topic:      requested.Topic,
				Partitions: partitions,
			})
		}
		return topics
	}

	topics := make([]protocol.OffsetFetchTopicResult, 0, len(result.Offsets))
	for _, name := range sortedTopicNames(result.Offsets) {
		fetched := result.Offsets[name]
		partitions := make([]protocol.OffsetFetchPartition, 0, len(fetched))
		for _, partition := range sortedPartitionIDs(fetched) {
			partitions = append(partitions, toOffsetFetchPartition(partition, fetched[partition]))
		}
		topics = append(topics, protocol.OffsetFetchTopicResult{Topic: name, Partitions: partitions})
	}
	return topics
}

// toOffsetFetchPartition converts one fetched offset to wire form. A partition
// the coordinator returned nothing for reports OffsetNoCommittedValue, which
// the consumer must distinguish from a genuine committed offset of 0.
func toOffsetFetchPartition(partition int32, data group.OffsetFetchData) protocol.OffsetFetchPartition {
	offset := data.Offset
	if data.ErrorCode == group.ErrorCodeNone && offset < 0 {
		offset = protocol.OffsetNoCommittedValue
	}
	return protocol.OffsetFetchPartition{
		Partition: partition,
		Offset:    offset,
		Metadata:  data.Metadata,
		ErrorCode: groupErrorToProtocol(data.ErrorCode),
	}
}

// groupErrorToProtocol maps a coordinator error code onto its protocol
// counterpart. An unrecognised non-zero code becomes ErrInvalidRequest rather
// than being reported as success, so a new coordinator error can never reach a
// client looking like ErrNone.
func groupErrorToProtocol(code int16) protocol.ErrorCode {
	switch code {
	case group.ErrorCodeNone:
		return protocol.ErrNone
	case group.ErrorCodeIllegalGeneration:
		return protocol.ErrIllegalGeneration
	case group.ErrorCodeUnknownMemberID:
		return protocol.ErrUnknownMemberID
	case group.ErrorCodeRebalanceInProgress:
		return protocol.ErrRebalanceInProgress
	case group.ErrorCodeInvalidSessionTimeout:
		return protocol.ErrInvalidSessionTimeout
	case group.ErrorCodeGroupIDNotFound:
		return protocol.ErrUnknownConsumerGroupID
	case group.ErrorCodeGroupAuthorizationFailed:
		return protocol.ErrGroupAuthorizationFailed
	default:
		return protocol.ErrInvalidRequest
	}
}

// sortedTopicNames returns topic names in sorted order.
func sortedTopicNames(offsets map[string]map[int32]group.OffsetFetchData) []string {
	names := make([]string, 0, len(offsets))
	for name := range offsets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sortedPartitionIDs returns partition IDs in ascending order.
func sortedPartitionIDs(partitions map[int32]group.OffsetFetchData) []int32 {
	ids := make([]int32, 0, len(partitions))
	for id := range partitions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
