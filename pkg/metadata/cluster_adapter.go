package metadata

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shawntherrien/streambus/pkg/cluster"
)

// ClusterMetadataStore is an adapter that implements cluster.MetadataStore
// using the Raft-backed metadata.Store
type ClusterMetadataStore struct {
	store *Store
}

// NewClusterMetadataStore creates a new adapter for cluster metadata operations
func NewClusterMetadataStore(store *Store) *ClusterMetadataStore {
	return &ClusterMetadataStore{
		store: store,
	}
}

// StoreBrokerMetadata stores or updates broker metadata via Raft consensus
func (cms *ClusterMetadataStore) StoreBrokerMetadata(ctx context.Context, broker *cluster.BrokerMetadata) error {
	// Convert cluster.BrokerMetadata to metadata.BrokerInfo
	brokerInfo := cms.clusterBrokerToMetadata(broker)

	// Check if broker already exists
	_, exists := cms.store.GetBroker(brokerInfo.ID)
	if !exists {
		// Register new broker
		return cms.store.RegisterBroker(ctx, brokerInfo)
	}

	// Update existing broker
	return cms.store.UpdateBroker(ctx, brokerInfo.ID, brokerInfo.Status, brokerInfo.Resources)
}

// GetBrokerMetadata retrieves broker metadata
func (cms *ClusterMetadataStore) GetBrokerMetadata(ctx context.Context, brokerID int32) (*cluster.BrokerMetadata, error) {
	brokerInfo, exists := cms.store.GetBroker(uint64(brokerID))
	if !exists {
		return nil, fmt.Errorf("broker %d not found", brokerID)
	}

	return cms.metadataBrokerToCluster(brokerInfo), nil
}

// ListBrokers lists all brokers
func (cms *ClusterMetadataStore) ListBrokers(ctx context.Context) ([]*cluster.BrokerMetadata, error) {
	brokers := cms.store.ListBrokers()

	result := make([]*cluster.BrokerMetadata, len(brokers))
	for i, brokerInfo := range brokers {
		result[i] = cms.metadataBrokerToCluster(brokerInfo)
	}

	return result, nil
}

// DeleteBroker removes broker metadata
func (cms *ClusterMetadataStore) DeleteBroker(ctx context.Context, brokerID int32) error {
	return cms.store.UnregisterBroker(ctx, uint64(brokerID))
}

// clusterBrokerToMetadata converts cluster.BrokerMetadata to metadata.BrokerInfo
func (cms *ClusterMetadataStore) clusterBrokerToMetadata(broker *cluster.BrokerMetadata) *BrokerInfo {
	// Convert status
	var status BrokerStatus
	switch broker.Status {
	case cluster.BrokerStatusStarting:
		status = BrokerStatusAlive // Starting maps to alive in metadata
	case cluster.BrokerStatusAlive:
		status = BrokerStatusAlive
	case cluster.BrokerStatusFailed:
		status = BrokerStatusDead
	case cluster.BrokerStatusDraining:
		status = BrokerStatusDraining
	case cluster.BrokerStatusDecommissioned:
		status = BrokerStatusDead // Decommissioned maps to dead
	default:
		status = BrokerStatusUnreachable
	}

	return &BrokerInfo{
		ID:     uint64(broker.ID),
		Addr:   broker.Address(),
		Status: status,
		Resources: BrokerResources{
			DiskTotal:        uint64(broker.DiskCapacityGB) * 1024 * 1024 * 1024, // Convert GB to bytes
			DiskUsed:         uint64(broker.DiskUsedGB) * 1024 * 1024 * 1024,
			NetworkBandwidth: uint64(broker.NetworkBandwidth),
		},
		Rack:         broker.Rack,
		RegisteredAt: broker.RegisteredAt,
	}
}

// metadataBrokerToCluster converts metadata.BrokerInfo to cluster.BrokerMetadata
func (cms *ClusterMetadataStore) metadataBrokerToCluster(brokerInfo *BrokerInfo) *cluster.BrokerMetadata {
	// Parse address into host and port
	host, port := cms.parseAddress(brokerInfo.Addr)

	// Convert status
	var status cluster.BrokerStatus
	switch brokerInfo.Status {
	case BrokerStatusAlive:
		status = cluster.BrokerStatusAlive
	case BrokerStatusDead:
		status = cluster.BrokerStatusFailed
	case BrokerStatusDraining:
		status = cluster.BrokerStatusDraining
	case BrokerStatusUnreachable:
		status = cluster.BrokerStatusFailed
	default:
		status = cluster.BrokerStatusFailed // Default to failed for unknown
	}

	return &cluster.BrokerMetadata{
		ID:               int32(brokerInfo.ID),
		Host:             host,
		Port:             port,
		Rack:             brokerInfo.Rack,
		Status:           status,
		RegisteredAt:     brokerInfo.RegisteredAt,
		LastHeartbeat:    time.Now(), // Use current time as approximation
		DiskCapacityGB:   int64(brokerInfo.Resources.DiskTotal / (1024 * 1024 * 1024)),    // Convert bytes to GB
		DiskUsedGB:       int64(brokerInfo.Resources.DiskUsed / (1024 * 1024 * 1024)),     // Convert bytes to GB
		NetworkBandwidth: int64(brokerInfo.Resources.NetworkBandwidth),
		Tags:             make(map[string]string), // metadata.BrokerInfo doesn't have Tags, use empty map
	}
}

// parseAddress parses "host:port" into host and port
func (cms *ClusterMetadataStore) parseAddress(addr string) (string, int) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return addr, 0
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return addr, 0
	}

	return parts[0], port
}

// StorePartitionAssignment stores partition assignment via Raft consensus
func (cms *ClusterMetadataStore) StorePartitionAssignment(ctx context.Context, assignment *cluster.Assignment) error {
	// Create partitions based on assignment
	var partitions []*PartitionInfo

	for key, replicas := range assignment.Partitions {
		topic, partitionID := cms.parsePartitionKey(key)
		leader := assignment.Leaders[key]

		partition := &PartitionInfo{
			Topic:       topic,
			Partition:   partitionID,
			Replicas:    cms.int32ToUint64(replicas),
			ISR:         cms.int32ToUint64(replicas), // Initially all replicas in ISR
			Leader:      uint64(leader),
			LeaderEpoch: uint64(assignment.Version),
		}

		partitions = append(partitions, partition)
	}

	// Batch create partitions
	return cms.store.BatchCreatePartitions(ctx, partitions)
}

// GetPartitionAssignment retrieves partition assignment
func (cms *ClusterMetadataStore) GetPartitionAssignment(ctx context.Context, topic string) (*cluster.Assignment, error) {
	partitions := cms.store.ListPartitions(topic)

	assignment := cluster.NewAssignment()

	for _, partition := range partitions {
		key := cluster.PartitionKey(partition.Topic, partition.Partition)
		assignment.Partitions[key] = cms.uint64ToInt32(partition.Replicas)
		assignment.Leaders[key] = int32(partition.Leader)
	}

	// Recompute broker load
	assignment.RecomputeBrokerLoad()

	return assignment, nil
}

// parsePartitionKey parses "topic:partition" into topic and partition ID
func (cms *ClusterMetadataStore) parsePartitionKey(key string) (string, int) {
	idx := strings.LastIndex(key, ":")
	if idx == -1 {
		return key, 0
	}

	topic := key[:idx]
	partitionStr := key[idx+1:]

	partitionID, err := strconv.Atoi(partitionStr)
	if err != nil {
		return key, 0
	}

	return topic, partitionID
}

// int32ToUint64 converts []int32 to []uint64
func (cms *ClusterMetadataStore) int32ToUint64(in []int32) []uint64 {
	out := make([]uint64, len(in))
	for i, v := range in {
		out[i] = uint64(v)
	}
	return out
}

// uint64ToInt32 converts []uint64 to []int32
func (cms *ClusterMetadataStore) uint64ToInt32(in []uint64) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}
