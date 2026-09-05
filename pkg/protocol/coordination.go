package protocol

// Wire types for consumer group coordination.
//
// These mirror the coordinator-side request/response structs in
// pkg/consumer/group, but are defined here so the wire format is owned by the
// protocol package: changing an internal coordinator struct must not silently
// change what goes on the wire.
//
// Every payload below implements encodePayload/decodePayload against the
// payloadWriter/payloadReader helpers in wire.go. A payloadWriter with no
// buffer measures instead of writing, so encodePayload doubles as the size
// calculation and the two cannot drift apart.

// coordinationVersion is the version stamped into subscription and assignment
// blobs so the format can evolve without breaking older peers.
const coordinationVersion int16 = 1

// ---------------------------------------------------------------------------
// Subscription and assignment blobs
// ---------------------------------------------------------------------------

// Subscription is a consumer's declared interest, carried as opaque protocol
// metadata through JoinGroup.
type Subscription struct {
	// Topics the member wants to consume.
	Topics []string
	// UserData is passed through untouched for custom assignors.
	UserData []byte
}

// EncodeSubscription serializes a subscription into protocol metadata bytes.
func EncodeSubscription(sub *Subscription) []byte {
	sizer := newSizer()
	writeSubscription(sizer, sub)

	buf := make([]byte, sizer.Len())
	writeSubscription(newWriter(buf, 0), sub)
	return buf
}

func writeSubscription(w *payloadWriter, sub *Subscription) {
	w.writeInt16(coordinationVersion)
	w.writeArrayLen(len(sub.Topics))
	for _, topic := range sub.Topics {
		w.writeString(topic)
	}
	w.writeBytes(sub.UserData)
}

// DecodeSubscription parses protocol metadata bytes into a Subscription.
func DecodeSubscription(data []byte) (*Subscription, error) {
	r := newReader(data)

	r.readInt16() // version, reserved for future format changes
	count := r.readCount()
	if r.Err() != nil {
		return nil, r.Err()
	}

	sub := &Subscription{Topics: make([]string, 0, count)}
	for i := 0; i < count; i++ {
		sub.Topics = append(sub.Topics, r.readString())
	}
	sub.UserData = r.readBytes()

	if err := r.Err(); err != nil {
		return nil, err
	}
	return sub, nil
}

// MemberAssignment is the set of partitions assigned to one group member,
// carried as an opaque blob through SyncGroup.
type MemberAssignment struct {
	// Partitions assigned, keyed by topic.
	Partitions map[string][]int32
	// UserData is passed through untouched for custom assignors.
	UserData []byte
}

// EncodeMemberAssignment serializes a member assignment.
func EncodeMemberAssignment(assignment *MemberAssignment) []byte {
	sizer := newSizer()
	writeMemberAssignment(sizer, assignment)

	buf := make([]byte, sizer.Len())
	writeMemberAssignment(newWriter(buf, 0), assignment)
	return buf
}

func writeMemberAssignment(w *payloadWriter, assignment *MemberAssignment) {
	w.writeInt16(coordinationVersion)
	w.writeArrayLen(len(assignment.Partitions))

	// Sort topics so the same assignment always produces identical bytes;
	// callers compare assignment blobs to detect whether a rebalance actually
	// changed anything.
	for _, topic := range sortedKeys(assignment.Partitions) {
		w.writeString(topic)
		partitions := assignment.Partitions[topic]
		w.writeArrayLen(len(partitions))
		for _, partition := range partitions {
			w.writeInt32(partition)
		}
	}

	w.writeBytes(assignment.UserData)
}

// DecodeMemberAssignment parses an assignment blob.
func DecodeMemberAssignment(data []byte) (*MemberAssignment, error) {
	r := newReader(data)

	r.readInt16() // version
	topicCount := r.readCount()
	if r.Err() != nil {
		return nil, r.Err()
	}

	assignment := &MemberAssignment{
		Partitions: make(map[string][]int32, topicCount),
	}
	for i := 0; i < topicCount; i++ {
		topic := r.readString()
		partitionCount := r.readCount()
		if r.Err() != nil {
			return nil, r.Err()
		}
		partitions := make([]int32, 0, partitionCount)
		for j := 0; j < partitionCount; j++ {
			partitions = append(partitions, r.readInt32())
		}
		assignment.Partitions[topic] = partitions
	}
	assignment.UserData = r.readBytes()

	if err := r.Err(); err != nil {
		return nil, err
	}
	return assignment, nil
}

// ---------------------------------------------------------------------------
// JoinGroup
// ---------------------------------------------------------------------------

// GroupProtocol is one assignment protocol a member supports, with the
// subscription metadata to use if it is selected.
type GroupProtocol struct {
	Name     string
	Metadata []byte
}

// JoinGroupRequest asks the coordinator to admit a member to a group.
type JoinGroupRequest struct {
	GroupID            string
	MemberID           string // empty when joining for the first time
	ClientID           string
	ProtocolType       string
	SessionTimeoutMs   int32
	RebalanceTimeoutMs int32
	Protocols          []GroupProtocol
}

func (p *JoinGroupRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.GroupID)
	w.writeString(p.MemberID)
	w.writeString(p.ClientID)
	w.writeString(p.ProtocolType)
	w.writeInt32(p.SessionTimeoutMs)
	w.writeInt32(p.RebalanceTimeoutMs)
	w.writeArrayLen(len(p.Protocols))
	for _, protocol := range p.Protocols {
		w.writeString(protocol.Name)
		w.writeBytes(protocol.Metadata)
	}
}

func (p *JoinGroupRequest) decodePayload(r *payloadReader) {
	p.GroupID = r.readString()
	p.MemberID = r.readString()
	p.ClientID = r.readString()
	p.ProtocolType = r.readString()
	p.SessionTimeoutMs = r.readInt32()
	p.RebalanceTimeoutMs = r.readInt32()

	count := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Protocols = make([]GroupProtocol, 0, count)
	for i := 0; i < count; i++ {
		p.Protocols = append(p.Protocols, GroupProtocol{
			Name:     r.readString(),
			Metadata: r.readBytes(),
		})
	}
}

// JoinGroupMember describes one member of the group. Only the elected leader
// receives the full list; every other member gets an empty one.
type JoinGroupMember struct {
	MemberID string
	Metadata []byte
}

// JoinGroupResponse reports the outcome of a join.
type JoinGroupResponse struct {
	ErrorCode    ErrorCode
	GenerationID int32
	ProtocolName string
	MemberID     string
	LeaderID     string
	Members      []JoinGroupMember
}

// IsLeader reports whether this member was elected group leader, and is
// therefore responsible for computing the assignment.
func (p *JoinGroupResponse) IsLeader() bool {
	return p.MemberID != "" && p.MemberID == p.LeaderID
}

func (p *JoinGroupResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
	w.writeInt32(p.GenerationID)
	w.writeString(p.ProtocolName)
	w.writeString(p.MemberID)
	w.writeString(p.LeaderID)
	w.writeArrayLen(len(p.Members))
	for _, member := range p.Members {
		w.writeString(member.MemberID)
		w.writeBytes(member.Metadata)
	}
}

func (p *JoinGroupResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
	p.GenerationID = r.readInt32()
	p.ProtocolName = r.readString()
	p.MemberID = r.readString()
	p.LeaderID = r.readString()

	count := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Members = make([]JoinGroupMember, 0, count)
	for i := 0; i < count; i++ {
		p.Members = append(p.Members, JoinGroupMember{
			MemberID: r.readString(),
			Metadata: r.readBytes(),
		})
	}
}

// ---------------------------------------------------------------------------
// SyncGroup
// ---------------------------------------------------------------------------

// SyncGroupAssignment is one member's assignment as computed by the leader.
type SyncGroupAssignment struct {
	MemberID   string
	Assignment []byte
}

// SyncGroupRequest collects the leader's assignment and hands each member its
// own share. Non-leaders send an empty Assignments list.
type SyncGroupRequest struct {
	GroupID      string
	GenerationID int32
	MemberID     string
	Assignments  []SyncGroupAssignment
}

func (p *SyncGroupRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.GroupID)
	w.writeInt32(p.GenerationID)
	w.writeString(p.MemberID)
	w.writeArrayLen(len(p.Assignments))
	for _, assignment := range p.Assignments {
		w.writeString(assignment.MemberID)
		w.writeBytes(assignment.Assignment)
	}
}

func (p *SyncGroupRequest) decodePayload(r *payloadReader) {
	p.GroupID = r.readString()
	p.GenerationID = r.readInt32()
	p.MemberID = r.readString()

	count := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Assignments = make([]SyncGroupAssignment, 0, count)
	for i := 0; i < count; i++ {
		p.Assignments = append(p.Assignments, SyncGroupAssignment{
			MemberID:   r.readString(),
			Assignment: r.readBytes(),
		})
	}
}

// SyncGroupResponse carries the calling member's assignment.
type SyncGroupResponse struct {
	ErrorCode  ErrorCode
	Assignment []byte
}

func (p *SyncGroupResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
	w.writeBytes(p.Assignment)
}

func (p *SyncGroupResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
	p.Assignment = r.readBytes()
}

// ---------------------------------------------------------------------------
// Heartbeat
// ---------------------------------------------------------------------------

// HeartbeatRequest keeps a member's session alive.
type HeartbeatRequest struct {
	GroupID      string
	GenerationID int32
	MemberID     string
}

func (p *HeartbeatRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.GroupID)
	w.writeInt32(p.GenerationID)
	w.writeString(p.MemberID)
}

func (p *HeartbeatRequest) decodePayload(r *payloadReader) {
	p.GroupID = r.readString()
	p.GenerationID = r.readInt32()
	p.MemberID = r.readString()
}

// HeartbeatResponse reports whether the member is still a valid group member.
// ErrRebalanceInProgress tells the member to rejoin.
type HeartbeatResponse struct {
	ErrorCode ErrorCode
}

func (p *HeartbeatResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
}

func (p *HeartbeatResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
}

// ---------------------------------------------------------------------------
// LeaveGroup
// ---------------------------------------------------------------------------

// LeaveGroupRequest removes a member from its group.
type LeaveGroupRequest struct {
	GroupID  string
	MemberID string
}

func (p *LeaveGroupRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.GroupID)
	w.writeString(p.MemberID)
}

func (p *LeaveGroupRequest) decodePayload(r *payloadReader) {
	p.GroupID = r.readString()
	p.MemberID = r.readString()
}

// LeaveGroupResponse reports the outcome of a leave.
type LeaveGroupResponse struct {
	ErrorCode ErrorCode
}

func (p *LeaveGroupResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
}

func (p *LeaveGroupResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
}

// ---------------------------------------------------------------------------
// OffsetCommit / OffsetFetch
// ---------------------------------------------------------------------------

// OffsetCommitPartition is one partition's committed position.
type OffsetCommitPartition struct {
	Partition int32
	Offset    int64
	Metadata  string
}

// OffsetCommitTopic groups committed partitions by topic.
type OffsetCommitTopic struct {
	Topic      string
	Partitions []OffsetCommitPartition
}

// OffsetCommitRequest stores consumer positions in the group.
//
// GenerationID and MemberID identify the committing member; a simple consumer
// outside a generation sends -1 and "".
type OffsetCommitRequest struct {
	GroupID      string
	GenerationID int32
	MemberID     string
	Topics       []OffsetCommitTopic
}

func (p *OffsetCommitRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.GroupID)
	w.writeInt32(p.GenerationID)
	w.writeString(p.MemberID)
	w.writeArrayLen(len(p.Topics))
	for _, topic := range p.Topics {
		w.writeString(topic.Topic)
		w.writeArrayLen(len(topic.Partitions))
		for _, partition := range topic.Partitions {
			w.writeInt32(partition.Partition)
			w.writeInt64(partition.Offset)
			w.writeString(partition.Metadata)
		}
	}
}

func (p *OffsetCommitRequest) decodePayload(r *payloadReader) {
	p.GroupID = r.readString()
	p.GenerationID = r.readInt32()
	p.MemberID = r.readString()

	topicCount := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Topics = make([]OffsetCommitTopic, 0, topicCount)
	for i := 0; i < topicCount; i++ {
		topic := OffsetCommitTopic{Topic: r.readString()}
		partitionCount := r.readCount()
		if r.Err() != nil {
			return
		}
		topic.Partitions = make([]OffsetCommitPartition, 0, partitionCount)
		for j := 0; j < partitionCount; j++ {
			topic.Partitions = append(topic.Partitions, OffsetCommitPartition{
				Partition: r.readInt32(),
				Offset:    r.readInt64(),
				Metadata:  r.readString(),
			})
		}
		p.Topics = append(p.Topics, topic)
	}
}

// OffsetCommitPartitionResult is the per-partition outcome of a commit.
type OffsetCommitPartitionResult struct {
	Partition int32
	ErrorCode ErrorCode
}

// OffsetCommitTopicResult groups commit results by topic.
type OffsetCommitTopicResult struct {
	Topic      string
	Partitions []OffsetCommitPartitionResult
}

// OffsetCommitResponse reports per-partition commit outcomes. A partition
// missing from the response was not acted on.
type OffsetCommitResponse struct {
	Topics []OffsetCommitTopicResult
}

// FirstError returns the first non-zero error code in the response, or ErrNone
// when every partition committed successfully.
func (p *OffsetCommitResponse) FirstError() ErrorCode {
	for _, topic := range p.Topics {
		for _, partition := range topic.Partitions {
			if partition.ErrorCode != ErrNone {
				return partition.ErrorCode
			}
		}
	}
	return ErrNone
}

func (p *OffsetCommitResponse) encodePayload(w *payloadWriter) {
	w.writeArrayLen(len(p.Topics))
	for _, topic := range p.Topics {
		w.writeString(topic.Topic)
		w.writeArrayLen(len(topic.Partitions))
		for _, partition := range topic.Partitions {
			w.writeInt32(partition.Partition)
			w.writeErrorCode(partition.ErrorCode)
		}
	}
}

func (p *OffsetCommitResponse) decodePayload(r *payloadReader) {
	topicCount := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Topics = make([]OffsetCommitTopicResult, 0, topicCount)
	for i := 0; i < topicCount; i++ {
		topic := OffsetCommitTopicResult{Topic: r.readString()}
		partitionCount := r.readCount()
		if r.Err() != nil {
			return
		}
		topic.Partitions = make([]OffsetCommitPartitionResult, 0, partitionCount)
		for j := 0; j < partitionCount; j++ {
			topic.Partitions = append(topic.Partitions, OffsetCommitPartitionResult{
				Partition: r.readInt32(),
				ErrorCode: r.readErrorCode(),
			})
		}
		p.Topics = append(p.Topics, topic)
	}
}

// OffsetFetchTopic names the partitions to fetch offsets for.
type OffsetFetchTopic struct {
	Topic      string
	Partitions []int32
}

// OffsetFetchRequest retrieves committed positions for a group. An empty
// Topics list fetches every topic the group has committed offsets for.
type OffsetFetchRequest struct {
	GroupID string
	Topics  []OffsetFetchTopic
}

func (p *OffsetFetchRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.GroupID)
	w.writeArrayLen(len(p.Topics))
	for _, topic := range p.Topics {
		w.writeString(topic.Topic)
		w.writeArrayLen(len(topic.Partitions))
		for _, partition := range topic.Partitions {
			w.writeInt32(partition)
		}
	}
}

func (p *OffsetFetchRequest) decodePayload(r *payloadReader) {
	p.GroupID = r.readString()

	topicCount := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Topics = make([]OffsetFetchTopic, 0, topicCount)
	for i := 0; i < topicCount; i++ {
		topic := OffsetFetchTopic{Topic: r.readString()}
		partitionCount := r.readCount()
		if r.Err() != nil {
			return
		}
		topic.Partitions = make([]int32, 0, partitionCount)
		for j := 0; j < partitionCount; j++ {
			topic.Partitions = append(topic.Partitions, r.readInt32())
		}
		p.Topics = append(p.Topics, topic)
	}
}

// OffsetFetchPartition is one partition's committed position.
//
// An Offset of OffsetNoCommittedValue means the group has never committed for
// that partition, which is different from a committed offset of 0.
type OffsetFetchPartition struct {
	Partition int32
	Offset    int64
	Metadata  string
	ErrorCode ErrorCode
}

// OffsetNoCommittedValue is the offset reported for a partition the group has
// no committed offset for.
const OffsetNoCommittedValue int64 = -1

// OffsetFetchTopicResult groups fetched offsets by topic.
type OffsetFetchTopicResult struct {
	Topic      string
	Partitions []OffsetFetchPartition
}

// OffsetFetchResponse carries committed positions back to the consumer.
type OffsetFetchResponse struct {
	Topics []OffsetFetchTopicResult
}

func (p *OffsetFetchResponse) encodePayload(w *payloadWriter) {
	w.writeArrayLen(len(p.Topics))
	for _, topic := range p.Topics {
		w.writeString(topic.Topic)
		w.writeArrayLen(len(topic.Partitions))
		for _, partition := range topic.Partitions {
			w.writeInt32(partition.Partition)
			w.writeInt64(partition.Offset)
			w.writeString(partition.Metadata)
			w.writeErrorCode(partition.ErrorCode)
		}
	}
}

func (p *OffsetFetchResponse) decodePayload(r *payloadReader) {
	topicCount := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Topics = make([]OffsetFetchTopicResult, 0, topicCount)
	for i := 0; i < topicCount; i++ {
		topic := OffsetFetchTopicResult{Topic: r.readString()}
		partitionCount := r.readCount()
		if r.Err() != nil {
			return
		}
		topic.Partitions = make([]OffsetFetchPartition, 0, partitionCount)
		for j := 0; j < partitionCount; j++ {
			topic.Partitions = append(topic.Partitions, OffsetFetchPartition{
				Partition: r.readInt32(),
				Offset:    r.readInt64(),
				Metadata:  r.readString(),
				ErrorCode: r.readErrorCode(),
			})
		}
		p.Topics = append(p.Topics, topic)
	}
}
