package metadata

import (
	"encoding/json"
	"fmt"
	"time"
)

// ClusterMetadata represents all metadata for the StreamBus cluster.
// This is the root state that is replicated via Raft consensus.
type ClusterMetadata struct {
	// Brokers maps broker ID to broker metadata
	Brokers map[uint64]*BrokerInfo

	// Topics maps topic name to topic metadata
	Topics map[string]*TopicInfo

	// Partitions maps partition ID to partition metadata
	// Key format: "topic:partition" (e.g., "events:0")
	Partitions map[string]*PartitionInfo

	// Version is incremented on every change (for optimistic locking)
	Version uint64

	// LastModified is the timestamp of the last modification
	LastModified time.Time
}

// NewClusterMetadata creates a new empty cluster metadata.
func NewClusterMetadata() *ClusterMetadata {
	return &ClusterMetadata{
		Brokers:      make(map[uint64]*BrokerInfo),
		Topics:       make(map[string]*TopicInfo),
		Partitions:   make(map[string]*PartitionInfo),
		Version:      0,
		LastModified: time.Now(),
	}
}

// Clone creates a deep copy of the cluster metadata.
func (cm *ClusterMetadata) Clone() *ClusterMetadata {
	clone := &ClusterMetadata{
		Brokers:      make(map[uint64]*BrokerInfo),
		Topics:       make(map[string]*TopicInfo),
		Partitions:   make(map[string]*PartitionInfo),
		Version:      cm.Version,
		LastModified: cm.LastModified,
	}

	for id, broker := range cm.Brokers {
		clone.Brokers[id] = broker.Clone()
	}

	for name, topic := range cm.Topics {
		clone.Topics[name] = topic.Clone()
	}

	for id, partition := range cm.Partitions {
		clone.Partitions[id] = partition.Clone()
	}

	return clone
}

// BrokerInfo represents metadata about a broker in the cluster.
type BrokerInfo struct {
	// ID is the unique broker identifier
	ID uint64

	// Addr is the network address (host:port)
	Addr string

	// Status is the current broker status
	Status BrokerStatus

	// Resources contains resource information (CPU, disk, memory, network)
	Resources BrokerResources

	// Rack is the rack/availability zone (for rack-aware placement)
	Rack string

	// RegisteredAt is when the broker joined the cluster
	RegisteredAt time.Time

	// LastHeartbeat is the last time we received a heartbeat
	LastHeartbeat time.Time
}

// Clone creates a deep copy of the broker info.
func (bi *BrokerInfo) Clone() *BrokerInfo {
	if bi == nil {
		return nil
	}
	clone := *bi
	return &clone
}

// IsAlive returns true if the broker is considered alive.
func (bi *BrokerInfo) IsAlive(timeout time.Duration) bool {
	return bi.Status == BrokerStatusAlive &&
		time.Since(bi.LastHeartbeat) < timeout
}

// BrokerStatus represents the status of a broker.
type BrokerStatus string

const (
	BrokerStatusAlive       BrokerStatus = "alive"
	BrokerStatusDead        BrokerStatus = "dead"
	BrokerStatusDraining    BrokerStatus = "draining"
	BrokerStatusUnreachable BrokerStatus = "unreachable"
)

// BrokerResources represents resource information for a broker.
type BrokerResources struct {
	// DiskTotal is total disk space in bytes
	DiskTotal uint64

	// DiskUsed is used disk space in bytes
	DiskUsed uint64

	// DiskAvailable is available disk space in bytes
	DiskAvailable uint64

	// NetworkBandwidth is network bandwidth in bytes/sec
	NetworkBandwidth uint64

	// CPUCores is the number of CPU cores
	CPUCores int

	// MemoryTotal is total memory in bytes
	MemoryTotal uint64

	// MemoryUsed is used memory in bytes
	MemoryUsed uint64
}

// TopicInfo represents metadata about a topic.
type TopicInfo struct {
	// Name is the topic name
	Name string

	// NumPartitions is the number of partitions
	NumPartitions int

	// ReplicationFactor is the replication factor
	ReplicationFactor int

	// Config contains topic-specific configuration
	Config TopicConfig

	// CreatedAt is when the topic was created
	CreatedAt time.Time

	// ModifiedAt is when the topic was last modified
	ModifiedAt time.Time
}

// Clone creates a deep copy of the topic info.
func (ti *TopicInfo) Clone() *TopicInfo {
	if ti == nil {
		return nil
	}
	clone := *ti
	clone.Config = ti.Config.Clone()
	return &clone
}

// TopicConfig contains topic-specific configuration.
type TopicConfig struct {
	// RetentionMs is the retention time in milliseconds
	RetentionMs int64

	// RetentionBytes is the retention size in bytes per partition
	RetentionBytes int64

	// SegmentMs is the segment roll time in milliseconds
	SegmentMs int64

	// SegmentBytes is the segment size in bytes
	SegmentBytes int64

	// MinInsyncReplicas is the minimum number of in-sync replicas
	MinInsyncReplicas int

	// CompressionType is the compression type (none, gzip, snappy, lz4, zstd)
	CompressionType string
}

// Clone creates a copy of the topic config.
func (tc TopicConfig) Clone() TopicConfig {
	return tc
}

// DefaultTopicConfig returns default topic configuration.
func DefaultTopicConfig() TopicConfig {
	return TopicConfig{
		RetentionMs:       7 * 24 * 60 * 60 * 1000, // 7 days
		RetentionBytes:    -1,                       // Unlimited
		SegmentMs:         7 * 24 * 60 * 60 * 1000, // 7 days
		SegmentBytes:      1024 * 1024 * 1024,      // 1GB
		MinInsyncReplicas: 1,
		CompressionType:   "none",
	}
}

// PartitionInfo represents metadata about a partition.
type PartitionInfo struct {
	// Topic is the topic name
	Topic string

	// Partition is the partition number
	Partition int

	// Leader is the broker ID of the leader (0 if no leader)
	Leader uint64

	// Replicas is the list of all replica broker IDs
	Replicas []uint64

	// ISR is the list of in-sync replica broker IDs
	ISR []uint64

	// OfflineReplicas is the list of offline replica broker IDs
	OfflineReplicas []uint64

	// LeaderEpoch is incremented each time leadership changes
	LeaderEpoch uint64

	// CreatedAt is when the partition was created
	CreatedAt time.Time

	// ModifiedAt is when the partition was last modified
	ModifiedAt time.Time
}

// Clone creates a deep copy of the partition info.
func (pi *PartitionInfo) Clone() *PartitionInfo {
	if pi == nil {
		return nil
	}
	clone := *pi
	clone.Replicas = append([]uint64(nil), pi.Replicas...)
	clone.ISR = append([]uint64(nil), pi.ISR...)
	clone.OfflineReplicas = append([]uint64(nil), pi.OfflineReplicas...)
	return &clone
}

// PartitionID returns the partition identifier (topic:partition).
func (pi *PartitionInfo) PartitionID() string {
	return PartitionID(pi.Topic, pi.Partition)
}

// IsLeader returns true if the given broker is the leader.
func (pi *PartitionInfo) IsLeader(brokerID uint64) bool {
	return pi.Leader == brokerID
}

// IsReplica returns true if the given broker is a replica.
func (pi *PartitionInfo) IsReplica(brokerID uint64) bool {
	for _, id := range pi.Replicas {
		if id == brokerID {
			return true
		}
	}
	return false
}

// IsInISR returns true if the given broker is in the ISR.
func (pi *PartitionInfo) IsInISR(brokerID uint64) bool {
	for _, id := range pi.ISR {
		if id == brokerID {
			return true
		}
	}
	return false
}

// PartitionID returns the partition identifier string.
func PartitionID(topic string, partition int) string {
	return fmt.Sprintf("%s:%d", topic, partition)
}

// Operation represents a metadata operation to be replicated via Raft.
type Operation struct {
	// Type is the operation type
	Type OperationType

	// Data is the operation-specific data (JSON encoded)
	Data []byte

	// Timestamp is when the operation was created
	Timestamp time.Time
}

// OperationType represents the type of metadata operation.
type OperationType string

const (
	// Broker operations
	OpRegisterBroker   OperationType = "register_broker"
	OpUnregisterBroker OperationType = "unregister_broker"
	OpUpdateBroker     OperationType = "update_broker"

	// Topic operations
	OpCreateTopic OperationType = "create_topic"
	OpDeleteTopic OperationType = "delete_topic"
	OpUpdateTopic OperationType = "update_topic"

	// Partition operations
	OpCreatePartition   OperationType = "create_partition"
	OpUpdatePartition   OperationType = "update_partition"
	OpUpdateLeader      OperationType = "update_leader"
	OpUpdateISR         OperationType = "update_isr"
	OpAddReplica        OperationType = "add_replica"
	OpRemoveReplica     OperationType = "remove_replica"
	OpUpdateReplicaList OperationType = "update_replica_list"

	// Batch operations
	OpBatchCreatePartitions OperationType = "batch_create_partitions"
)

// RegisterBrokerOp is the data for OpRegisterBroker.
type RegisterBrokerOp struct {
	Broker BrokerInfo
}

// UnregisterBrokerOp is the data for OpUnregisterBroker.
type UnregisterBrokerOp struct {
	BrokerID uint64
}

// UpdateBrokerOp is the data for OpUpdateBroker.
type UpdateBrokerOp struct {
	BrokerID  uint64
	Status    BrokerStatus
	Resources BrokerResources
	Heartbeat time.Time
}

// CreateTopicOp is the data for OpCreateTopic.
type CreateTopicOp struct {
	Name              string
	NumPartitions     int
	ReplicationFactor int
	Config            TopicConfig
}

// DeleteTopicOp is the data for OpDeleteTopic.
type DeleteTopicOp struct {
	Name string
}

// UpdateTopicOp is the data for OpUpdateTopic.
type UpdateTopicOp struct {
	Name   string
	Config TopicConfig
}

// CreatePartitionOp is the data for OpCreatePartition.
type CreatePartitionOp struct {
	Partition PartitionInfo
}

// UpdatePartitionOp is the data for OpUpdatePartition.
type UpdatePartitionOp struct {
	Partition PartitionInfo
}

// UpdateLeaderOp is the data for OpUpdateLeader.
type UpdateLeaderOp struct {
	Topic       string
	Partition   int
	Leader      uint64
	LeaderEpoch uint64
}

// UpdateISROp is the data for OpUpdateISR.
type UpdateISROp struct {
	Topic     string
	Partition int
	ISR       []uint64
}

// UpdateReplicaListOp is the data for OpUpdateReplicaList.
type UpdateReplicaListOp struct {
	Topic     string
	Partition int
	Replicas  []uint64
}

// BatchCreatePartitionsOp is the data for OpBatchCreatePartitions.
// This allows creating multiple partitions in a single Raft proposal,
// which is more efficient and prevents leadership instability.
type BatchCreatePartitionsOp struct {
	Partitions []PartitionInfo
}

// EncodeOperation encodes an operation to bytes.
func EncodeOperation(op Operation) ([]byte, error) {
	return json.Marshal(op)
}

// DecodeOperation decodes an operation from bytes.
func DecodeOperation(data []byte) (*Operation, error) {
	var op Operation
	if err := json.Unmarshal(data, &op); err != nil {
		return nil, err
	}
	return &op, nil
}
