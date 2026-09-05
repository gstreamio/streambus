package protocol

// FindCoordinator locates the broker responsible for a consumer group or a
// transactional ID, so a client no longer has to assume the first configured
// broker is always the coordinator.
//
// Follows the same self-describing payload style as coordination.go.

// CoordinatorKeyType distinguishes the two kinds of key a client can ask a
// coordinator for, since a group ID and a transactional ID are resolved
// independently even though they share the same wire request.
type CoordinatorKeyType int8

const (
	CoordinatorKeyTypeGroup       CoordinatorKeyType = 0
	CoordinatorKeyTypeTransaction CoordinatorKeyType = 1
)

// FindCoordinatorRequest asks any broker which broker coordinates Key.
type FindCoordinatorRequest struct {
	Key     string
	KeyType CoordinatorKeyType
}

func (p *FindCoordinatorRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.Key)
	w.writeInt8(int8(p.KeyType))
}

func (p *FindCoordinatorRequest) decodePayload(r *payloadReader) {
	p.Key = r.readString()
	p.KeyType = CoordinatorKeyType(r.readInt8())
}

// FindCoordinatorResponse names the coordinating broker. A non-zero ErrorCode
// means Node, Host and Port carry no meaning - e.g. ErrNotCoordinator when no
// broker is currently known to be live.
type FindCoordinatorResponse struct {
	ErrorCode ErrorCode
	NodeID    int32
	Host      string
	Port      int32
}

func (p *FindCoordinatorResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
	w.writeInt32(p.NodeID)
	w.writeString(p.Host)
	w.writeInt32(p.Port)
}

func (p *FindCoordinatorResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
	p.NodeID = r.readInt32()
	p.Host = r.readString()
	p.Port = r.readInt32()
}
