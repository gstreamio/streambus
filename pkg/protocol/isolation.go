package protocol

import "strconv"

// IsolationLevel controls which records a fetch can observe relative to
// in-flight transactions.
type IsolationLevel int8

const (
	// IsolationReadUncommitted returns every record up to the partition's
	// high water mark, including ones written by a transaction that has not
	// committed or aborted yet. This is the zero value, so it is also what a
	// client written before isolation levels existed sends.
	IsolationReadUncommitted IsolationLevel = 0

	// IsolationReadCommitted stops a fetch at the partition's last stable
	// offset (see Partition.LastStableOffset in pkg/server), so a record
	// from a transaction still in flight is never returned.
	//
	// It also retroactively hides the records of a transaction that has
	// already aborted: once the abort marker lifts the LastStableOffset
	// barrier, Partition consults the aborted transaction's remembered
	// offset range (see Partition.IsAborted) and its records' persisted
	// producer identity (Message.ProducerID/ProducerEpoch, storage record
	// format v3) to keep filtering them out of read-committed fetches
	// specifically - the same records remain visible under
	// IsolationReadUncommitted.
	IsolationReadCommitted IsolationLevel = 1
)

// Header keys identifying a transaction control record in a partition log.
//
// A marker is stored as a normal log record carrying these headers (see
// pkg/broker's logMarkerWriter), so it occupies an offset in the partition
// and is recovered with the rest of the log. They live in this package,
// rather than next to the code that writes them, because the fetch path
// (pkg/server) must recognize and hide them from every consumer regardless
// of isolation level, and both that path and the writer already depend on
// protocol.
const (
	// ControlHeaderKey marks a record as a control record; its value names
	// the kind of control record.
	ControlHeaderKey = "streambus.control"
	// ControlTypeTxnMarker is the ControlHeaderKey value for a transaction
	// commit or abort marker.
	ControlTypeTxnMarker = "txn-marker"
	// TxnCommitHeaderKey holds "true" for a commit marker, "false" for abort.
	TxnCommitHeaderKey = "streambus.txn.commit"
	// TxnProducerIDHeaderKey holds the producer ID the marker belongs to.
	TxnProducerIDHeaderKey = "streambus.txn.producer_id"
	// TxnProducerEpochHeaderKey holds the producer epoch.
	TxnProducerEpochHeaderKey = "streambus.txn.producer_epoch"
)

// IsControlRecord reports whether a record's headers mark it as an internal
// transaction control record rather than user data. Consumers must never
// receive these regardless of isolation level.
func IsControlRecord(headers map[string][]byte) bool {
	value, ok := headers[ControlHeaderKey]
	return ok && string(value) == ControlTypeTxnMarker
}

// ParseTransactionMarker extracts a transaction marker's producer identity
// and outcome from its headers, inverting TransactionMarkerHeaders. ok is
// false when headers is not a marker at all, or a header value fails to
// parse - which should never happen for a marker this broker wrote itself,
// but must not panic if it somehow does, since the one caller that needs
// this (partition transaction-state recovery, see pkg/server) runs over
// whatever is on disk at startup.
func ParseTransactionMarker(headers map[string][]byte) (producerID int64, producerEpoch int16, commit bool, ok bool) {
	if !IsControlRecord(headers) {
		return 0, 0, false, false
	}

	pid, err := strconv.ParseInt(string(headers[TxnProducerIDHeaderKey]), 10, 64)
	if err != nil {
		return 0, 0, false, false
	}
	epoch, err := strconv.ParseInt(string(headers[TxnProducerEpochHeaderKey]), 10, 16)
	if err != nil {
		return 0, 0, false, false
	}
	commit, err = strconv.ParseBool(string(headers[TxnCommitHeaderKey]))
	if err != nil {
		return 0, 0, false, false
	}

	return pid, int16(epoch), commit, true
}

// TransactionMarkerHeaders builds the header set that identifies a
// transaction marker record.
//
// Marker records are written by the broker and recognized by the fetch path,
// so the exact header set is protocol, not an implementation detail of
// either side - building it here keeps a writer from drifting out of step
// with IsControlRecord.
func TransactionMarkerHeaders(producerID int64, producerEpoch int16, commit bool) map[string][]byte {
	return map[string][]byte{
		ControlHeaderKey:          []byte(ControlTypeTxnMarker),
		TxnCommitHeaderKey:        []byte(strconv.FormatBool(commit)),
		TxnProducerIDHeaderKey:    []byte(strconv.FormatInt(producerID, 10)),
		TxnProducerEpochHeaderKey: []byte(strconv.FormatInt(int64(producerEpoch), 10)),
	}
}
