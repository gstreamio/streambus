package protocol

import "fmt"

// payloadEncoder is implemented by payloads that serialize themselves through
// a payloadWriter. Because a writer can run in measuring mode, one
// encodePayload implementation serves both the size calculation and the
// encoding, so the two cannot drift apart.
type payloadEncoder interface {
	encodePayload(w *payloadWriter)
}

// payloadDecoder is implemented by payloads that parse themselves from a
// payloadReader.
type payloadDecoder interface {
	decodePayload(r *payloadReader)
}

// newCoordinationRequest returns an empty request payload for a coordination
// or transaction request type, or nil if the type is not one of them.
func newCoordinationRequest(reqType RequestType) payloadDecoder {
	switch reqType {
	case RequestTypeJoinGroup:
		return &JoinGroupRequest{}
	case RequestTypeSyncGroup:
		return &SyncGroupRequest{}
	case RequestTypeHeartbeat:
		return &HeartbeatRequest{}
	case RequestTypeLeaveGroup:
		return &LeaveGroupRequest{}
	case RequestTypeOffsetCommit:
		return &OffsetCommitRequest{}
	case RequestTypeOffsetFetch:
		return &OffsetFetchRequest{}
	case RequestTypeInitProducerID:
		return &InitProducerIDRequest{}
	case RequestTypeAddPartitionsToTxn:
		return &AddPartitionsToTxnRequest{}
	case RequestTypeAddOffsetsToTxn:
		return &AddOffsetsToTxnRequest{}
	case RequestTypeEndTxn:
		return &EndTxnRequest{}
	case RequestTypeTxnOffsetCommit:
		return &TxnOffsetCommitRequest{}
	default:
		return nil
	}
}

// newCoordinationResponse returns an empty response payload for a coordination
// or transaction request type, or nil if the type is not one of them.
func newCoordinationResponse(reqType RequestType) payloadDecoder {
	switch reqType {
	case RequestTypeJoinGroup:
		return &JoinGroupResponse{}
	case RequestTypeSyncGroup:
		return &SyncGroupResponse{}
	case RequestTypeHeartbeat:
		return &HeartbeatResponse{}
	case RequestTypeLeaveGroup:
		return &LeaveGroupResponse{}
	case RequestTypeOffsetCommit:
		return &OffsetCommitResponse{}
	case RequestTypeOffsetFetch:
		return &OffsetFetchResponse{}
	case RequestTypeInitProducerID:
		return &InitProducerIDResponse{}
	case RequestTypeAddPartitionsToTxn:
		return &AddPartitionsToTxnResponse{}
	case RequestTypeAddOffsetsToTxn:
		return &AddOffsetsToTxnResponse{}
	case RequestTypeEndTxn:
		return &EndTxnResponse{}
	case RequestTypeTxnOffsetCommit:
		return &TxnOffsetCommitResponse{}
	default:
		return nil
	}
}

// IsCoordinationRequest reports whether a request type belongs to the consumer
// group or transaction coordination protocols.
func IsCoordinationRequest(reqType RequestType) bool {
	return newCoordinationRequest(reqType) != nil
}

// measurePayload returns the exact encoded size of a self-describing payload.
func measurePayload(p payloadEncoder) uint32 {
	sizer := newSizer()
	p.encodePayload(sizer)
	//nolint:gosec // payload sizes are bounded by the codec's max message size
	return uint32(sizer.Len())
}

// encodeSelfDescribing writes a self-describing payload into buf at offset and
// returns the new offset.
func encodeSelfDescribing(buf []byte, offset int, p payloadEncoder) int {
	w := newWriter(buf, offset)
	p.encodePayload(w)
	return w.Len()
}

// decodeCoordinationRequest parses a coordination request payload.
func decodeCoordinationRequest(buf []byte, reqType RequestType) (interface{}, error) {
	payload := newCoordinationRequest(reqType)
	if payload == nil {
		return nil, fmt.Errorf("not a coordination request type: %v", reqType)
	}

	r := newReader(buf)
	payload.decodePayload(r)
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("decoding %v request: %w", reqType, err)
	}
	return payload, nil
}

// decodeCoordinationResponse parses a coordination response payload.
func decodeCoordinationResponse(buf []byte, reqType RequestType) (interface{}, error) {
	payload := newCoordinationResponse(reqType)
	if payload == nil {
		return nil, fmt.Errorf("not a coordination request type: %v", reqType)
	}

	r := newReader(buf)
	payload.decodePayload(r)
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("decoding %v response: %w", reqType, err)
	}
	return payload, nil
}
