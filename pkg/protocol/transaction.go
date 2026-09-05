package protocol

// Wire types for transactional messaging.
//
// As with coordination.go, these mirror the coordinator-side structs in
// pkg/transaction but are owned by the protocol package so the wire format is
// independent of the coordinator's internals.

// ---------------------------------------------------------------------------
// InitProducerID
// ---------------------------------------------------------------------------

// InitProducerIDRequest claims (or reclaims) a producer ID for a transactional
// ID. Reclaiming bumps the epoch, fencing any older producer instance still
// running under the same transactional ID.
//
// A producer with no transactional ID sends an empty TransactionID and gets an
// idempotent-only producer ID back.
type InitProducerIDRequest struct {
	TransactionID        string
	TransactionTimeoutMs int32
}

func (p *InitProducerIDRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.TransactionID)
	w.writeInt32(p.TransactionTimeoutMs)
}

func (p *InitProducerIDRequest) decodePayload(r *payloadReader) {
	p.TransactionID = r.readString()
	p.TransactionTimeoutMs = r.readInt32()
}

// InitProducerIDResponse carries the assigned producer identity.
type InitProducerIDResponse struct {
	ProducerID    int64
	ProducerEpoch int16
	ErrorCode     ErrorCode
}

func (p *InitProducerIDResponse) encodePayload(w *payloadWriter) {
	w.writeInt64(p.ProducerID)
	w.writeInt16(p.ProducerEpoch)
	w.writeErrorCode(p.ErrorCode)
}

func (p *InitProducerIDResponse) decodePayload(r *payloadReader) {
	p.ProducerID = r.readInt64()
	p.ProducerEpoch = r.readInt16()
	p.ErrorCode = r.readErrorCode()
}

// ---------------------------------------------------------------------------
// AddPartitionsToTxn
// ---------------------------------------------------------------------------

// TxnPartition names one topic-partition taking part in a transaction.
type TxnPartition struct {
	Topic     string
	Partition int32
}

// AddPartitionsToTxnRequest registers partitions with the transaction before
// they are written to, so the coordinator knows where to write markers.
type AddPartitionsToTxnRequest struct {
	TransactionID string
	ProducerID    int64
	ProducerEpoch int16
	Partitions    []TxnPartition
}

func (p *AddPartitionsToTxnRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.TransactionID)
	w.writeInt64(p.ProducerID)
	w.writeInt16(p.ProducerEpoch)
	w.writeArrayLen(len(p.Partitions))
	for _, partition := range p.Partitions {
		w.writeString(partition.Topic)
		w.writeInt32(partition.Partition)
	}
}

func (p *AddPartitionsToTxnRequest) decodePayload(r *payloadReader) {
	p.TransactionID = r.readString()
	p.ProducerID = r.readInt64()
	p.ProducerEpoch = r.readInt16()

	count := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Partitions = make([]TxnPartition, 0, count)
	for i := 0; i < count; i++ {
		p.Partitions = append(p.Partitions, TxnPartition{
			Topic:     r.readString(),
			Partition: r.readInt32(),
		})
	}
}

// TxnPartitionResult is one partition's outcome.
type TxnPartitionResult struct {
	Topic     string
	Partition int32
	ErrorCode ErrorCode
}

// AddPartitionsToTxnResponse reports per-partition outcomes.
type AddPartitionsToTxnResponse struct {
	Results []TxnPartitionResult
}

// FirstError returns the first non-zero error code, or ErrNone.
func (p *AddPartitionsToTxnResponse) FirstError() ErrorCode {
	for _, result := range p.Results {
		if result.ErrorCode != ErrNone {
			return result.ErrorCode
		}
	}
	return ErrNone
}

func (p *AddPartitionsToTxnResponse) encodePayload(w *payloadWriter) {
	w.writeArrayLen(len(p.Results))
	for _, result := range p.Results {
		w.writeString(result.Topic)
		w.writeInt32(result.Partition)
		w.writeErrorCode(result.ErrorCode)
	}
}

func (p *AddPartitionsToTxnResponse) decodePayload(r *payloadReader) {
	count := r.readCount()
	if r.Err() != nil {
		return
	}
	p.Results = make([]TxnPartitionResult, 0, count)
	for i := 0; i < count; i++ {
		p.Results = append(p.Results, TxnPartitionResult{
			Topic:     r.readString(),
			Partition: r.readInt32(),
			ErrorCode: r.readErrorCode(),
		})
	}
}

// ---------------------------------------------------------------------------
// AddOffsetsToTxn
// ---------------------------------------------------------------------------

// AddOffsetsToTxnRequest brings a consumer group's offset partition into the
// transaction, so offset commits and produced records commit atomically.
type AddOffsetsToTxnRequest struct {
	TransactionID string
	ProducerID    int64
	ProducerEpoch int16
	GroupID       string
}

func (p *AddOffsetsToTxnRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.TransactionID)
	w.writeInt64(p.ProducerID)
	w.writeInt16(p.ProducerEpoch)
	w.writeString(p.GroupID)
}

func (p *AddOffsetsToTxnRequest) decodePayload(r *payloadReader) {
	p.TransactionID = r.readString()
	p.ProducerID = r.readInt64()
	p.ProducerEpoch = r.readInt16()
	p.GroupID = r.readString()
}

// AddOffsetsToTxnResponse reports the outcome.
type AddOffsetsToTxnResponse struct {
	ErrorCode ErrorCode
}

func (p *AddOffsetsToTxnResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
}

func (p *AddOffsetsToTxnResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
}

// ---------------------------------------------------------------------------
// TxnOffsetCommit
// ---------------------------------------------------------------------------

// TxnOffsetCommitRequest commits consumer offsets inside a transaction. The
// offsets only become visible once the transaction commits.
type TxnOffsetCommitRequest struct {
	TransactionID string
	GroupID       string
	ProducerID    int64
	ProducerEpoch int16
	Topics        []OffsetCommitTopic
}

func (p *TxnOffsetCommitRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.TransactionID)
	w.writeString(p.GroupID)
	w.writeInt64(p.ProducerID)
	w.writeInt16(p.ProducerEpoch)
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

func (p *TxnOffsetCommitRequest) decodePayload(r *payloadReader) {
	p.TransactionID = r.readString()
	p.GroupID = r.readString()
	p.ProducerID = r.readInt64()
	p.ProducerEpoch = r.readInt16()

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

// TxnOffsetCommitResponse reports per-partition outcomes, reusing the plain
// offset-commit result shape.
type TxnOffsetCommitResponse struct {
	Topics []OffsetCommitTopicResult
}

// FirstError returns the first non-zero error code, or ErrNone.
func (p *TxnOffsetCommitResponse) FirstError() ErrorCode {
	return (&OffsetCommitResponse{Topics: p.Topics}).FirstError()
}

func (p *TxnOffsetCommitResponse) encodePayload(w *payloadWriter) {
	(&OffsetCommitResponse{Topics: p.Topics}).encodePayload(w)
}

func (p *TxnOffsetCommitResponse) decodePayload(r *payloadReader) {
	inner := &OffsetCommitResponse{}
	inner.decodePayload(r)
	p.Topics = inner.Topics
}

// ---------------------------------------------------------------------------
// EndTxn
// ---------------------------------------------------------------------------

// EndTxnRequest commits or aborts a transaction.
type EndTxnRequest struct {
	TransactionID string
	ProducerID    int64
	ProducerEpoch int16
	// Commit is true to commit, false to abort.
	Commit bool
}

func (p *EndTxnRequest) encodePayload(w *payloadWriter) {
	w.writeString(p.TransactionID)
	w.writeInt64(p.ProducerID)
	w.writeInt16(p.ProducerEpoch)
	w.writeBool(p.Commit)
}

func (p *EndTxnRequest) decodePayload(r *payloadReader) {
	p.TransactionID = r.readString()
	p.ProducerID = r.readInt64()
	p.ProducerEpoch = r.readInt16()
	p.Commit = r.readBool()
}

// EndTxnResponse reports the outcome. A response with ErrNone means every
// transaction marker was durably written to the participating partitions.
type EndTxnResponse struct {
	ErrorCode ErrorCode
}

func (p *EndTxnResponse) encodePayload(w *payloadWriter) {
	w.writeErrorCode(p.ErrorCode)
}

func (p *EndTxnResponse) decodePayload(r *payloadReader) {
	p.ErrorCode = r.readErrorCode()
}
