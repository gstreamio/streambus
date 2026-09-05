package broker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/metadata"
	"github.com/gstreamio/streambus/pkg/replication/link"
	"github.com/gstreamio/streambus/pkg/security"
	"github.com/gstreamio/streambus/pkg/storage"
)

// registerAdminAPI registers admin management HTTP endpoints
func (b *Broker) registerAdminAPI(mux *http.ServeMux) {
	// Cluster endpoints
	mux.HandleFunc("/api/v1/cluster", b.handleClusterInfo)

	// Broker endpoints
	mux.HandleFunc("/api/v1/brokers", b.handleBrokerList)
	mux.HandleFunc("/api/v1/brokers/", b.handleBrokerDetail)

	// Topic endpoints
	mux.HandleFunc("/api/v1/topics", b.handleTopics)
	mux.HandleFunc("/api/v1/topics/", b.handleTopicOperations)

	// Consumer group endpoints
	mux.HandleFunc("/api/v1/consumer-groups", b.handleConsumerGroups)
	mux.HandleFunc("/api/v1/consumer-groups/", b.handleConsumerGroupOperations)

	// Replication endpoints
	mux.HandleFunc("/api/v1/replication/links", b.handleReplicationLinks)
	mux.HandleFunc("/api/v1/replication/links/", b.handleReplicationLinkOperations)

	// Security endpoints
	mux.HandleFunc("/api/v1/security/status", b.handleSecurityStatus)
	mux.HandleFunc("/api/v1/security/acls", b.handleSecurityACLs)
	mux.HandleFunc("/api/v1/security/acls/", b.handleSecurityACLOperations)
	mux.HandleFunc("/api/v1/security/users", b.handleSecurityUsers)
	mux.HandleFunc("/api/v1/security/users/", b.handleSecurityUserOperations)

	b.logger.Info("Registered admin management API endpoints")
}

// ==================== Cluster Endpoints ====================

// ClusterInfo represents cluster metadata
type ClusterInfo struct {
	ClusterID       string `json:"cluster_id"`
	ControllerID    int32  `json:"controller_id"`
	Version         string `json:"version"`
	TotalBrokers    int    `json:"total_brokers"`
	ActiveBrokers   int    `json:"active_brokers"`
	TotalTopics     int    `json:"total_topics"`
	TotalPartitions int    `json:"total_partitions"`
	Uptime          string `json:"uptime"`
}

// handleClusterInfo handles GET /api/v1/cluster
func (b *Broker) handleClusterInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get broker stats
	brokerStats := b.registry.GetBrokerStats()

	// Get topic count
	var totalTopics, totalPartitions int
	if b.metaStore != nil {
		topics := b.metaStore.ListTopics()
		totalTopics = len(topics)
		for _, topic := range topics {
			totalPartitions += topic.NumPartitions
		}
	}

	// Get leader info
	leaderID := int32(0)
	if b.metaStore != nil && b.metaStore.IsLeader() {
		leaderID = b.config.BrokerID
	}

	// Calculate uptime (use registry stats for now)
	uptime := "unknown"
	if len(b.registry.ListBrokers()) > 0 {
		brokers := b.registry.ListBrokers()
		for _, broker := range brokers {
			if broker.ID == b.config.BrokerID {
				uptime = time.Since(broker.RegisteredAt).String()
				break
			}
		}
	}

	info := ClusterInfo{
		ClusterID:       fmt.Sprintf("streambus-cluster-%d", leaderID),
		ControllerID:    leaderID,
		Version:         "1.0.0",
		TotalBrokers:    brokerStats.TotalBrokers,
		ActiveBrokers:   brokerStats.AliveBrokers,
		TotalTopics:     totalTopics,
		TotalPartitions: totalPartitions,
		Uptime:          uptime,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// ==================== Broker Endpoints ====================

// BrokerInfo represents broker metadata for API responses
type BrokerInfo struct {
	ID        int32            `json:"id"`
	Host      string           `json:"host"`
	Port      int              `json:"port"`
	Status    string           `json:"status"`
	Leader    bool             `json:"leader"`
	Version   string           `json:"version"`
	Uptime    string           `json:"uptime"`
	Resources *BrokerResources `json:"resources,omitempty"`
}

// BrokerResources represents broker resource usage
type BrokerResources struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskPercent   float64 `json:"disk_percent"`
	DiskUsedGB    int64   `json:"disk_used_gb"`
	DiskTotalGB   int64   `json:"disk_total_gb"`
}

// handleBrokerList handles GET /api/v1/brokers
func (b *Broker) handleBrokerList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	brokers := b.registry.ListBrokers()
	brokerInfos := make([]BrokerInfo, 0, len(brokers))

	isLeader := b.raftNode != nil && b.raftNode.IsLeader()

	for _, broker := range brokers {
		info := BrokerInfo{
			ID:      broker.ID,
			Host:    broker.Host,
			Port:    broker.Port,
			Status:  string(broker.Status),
			Leader:  isLeader && broker.ID == b.config.BrokerID,
			Version: broker.Version,
			Uptime:  time.Since(broker.RegisteredAt).String(),
		}
		brokerInfos = append(brokerInfos, info)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(brokerInfos)
}

// handleBrokerDetail handles GET /api/v1/brokers/:id
func (b *Broker) handleBrokerDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract broker ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/brokers/")
	brokerID, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		http.Error(w, "Invalid broker ID", http.StatusBadRequest)
		return
	}

	broker, err := b.registry.GetBroker(int32(brokerID))
	if err != nil {
		http.Error(w, "Broker not found", http.StatusNotFound)
		return
	}

	isLeader := b.raftNode != nil && b.raftNode.IsLeader() && broker.ID == b.config.BrokerID

	info := BrokerInfo{
		ID:      broker.ID,
		Host:    broker.Host,
		Port:    broker.Port,
		Status:  string(broker.Status),
		Leader:  isLeader,
		Version: broker.Version,
		Uptime:  time.Since(broker.RegisteredAt).String(),
		Resources: &BrokerResources{
			CPUPercent:    45.0, // TODO: Get real metrics
			MemoryPercent: 60.0,
			DiskPercent:   broker.DiskUtilization(),
			DiskUsedGB:    broker.DiskUsedGB,
			DiskTotalGB:   broker.DiskCapacityGB,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// ==================== Topic Endpoints ====================

// TopicResponse represents topic metadata for API responses
type TopicResponse struct {
	Name              string          `json:"name"`
	NumPartitions     int             `json:"num_partitions"`
	ReplicationFactor int             `json:"replication_factor"`
	Partitions        []PartitionInfo `json:"partitions,omitempty"`
}

// PartitionInfo represents partition metadata
type PartitionInfo struct {
	ID              int32   `json:"id"`
	Leader          int32   `json:"leader"`
	Replicas        []int32 `json:"replicas"`
	ISR             []int32 `json:"isr"`
	BeginningOffset int64   `json:"beginning_offset"`
	EndOffset       int64   `json:"end_offset"`
	MessageCount    int64   `json:"message_count"`
}

// handleTopics handles GET/POST /api/v1/topics
func (b *Broker) handleTopics(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.listTopics(w, r)
	case http.MethodPost:
		b.createTopic(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listTopics handles GET /api/v1/topics
func (b *Broker) listTopics(w http.ResponseWriter, r *http.Request) {
	if b.metaStore == nil {
		http.Error(w, "Metadata store not available", http.StatusServiceUnavailable)
		return
	}

	topics := b.metaStore.ListTopics()

	topicResponses := make([]TopicResponse, 0, len(topics))
	for _, topic := range topics {
		resp := TopicResponse{
			Name:              topic.Name,
			NumPartitions:     topic.NumPartitions,
			ReplicationFactor: topic.ReplicationFactor,
		}
		topicResponses = append(topicResponses, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(topicResponses)
}

// createTopic handles POST /api/v1/topics
func (b *Broker) createTopic(w http.ResponseWriter, r *http.Request) {
	if b.metaStore == nil {
		http.Error(w, "Metadata store not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name              string            `json:"name"`
		NumPartitions     int               `json:"num_partitions"`
		ReplicationFactor int               `json:"replication_factor"`
		Config            map[string]string `json:"config,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Topic name is required", http.StatusBadRequest)
		return
	}

	if req.NumPartitions <= 0 {
		req.NumPartitions = 1
	}

	if req.ReplicationFactor <= 0 {
		req.ReplicationFactor = 1
	}

	// Create topic via metadata store
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use default topic config - could be extended to parse config from request
	topicConfig := metadata.DefaultTopicConfig()
	err := b.metaStore.CreateTopic(ctx, req.Name, req.NumPartitions, req.ReplicationFactor, topicConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create topic: %v", err), http.StatusInternalServerError)
		return
	}

	b.logger.Info("Topic created via API", logging.Fields{
		"topic":              req.Name,
		"num_partitions":     req.NumPartitions,
		"replication_factor": req.ReplicationFactor,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name":               req.Name,
		"num_partitions":     req.NumPartitions,
		"replication_factor": req.ReplicationFactor,
	})
}

// handleTopicOperations handles topic-specific operations
func (b *Broker) handleTopicOperations(w http.ResponseWriter, r *http.Request) {
	// Extract topic name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/topics/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Topic name required", http.StatusBadRequest)
		return
	}

	topicName := parts[0]

	// Check for sub-resources
	if len(parts) > 1 {
		switch parts[1] {
		case "partitions":
			b.handleTopicPartitions(w, r, topicName)
			return
		case "messages":
			b.handleTopicMessages(w, r, topicName)
			return
		}
	}

	// Handle topic operations
	switch r.Method {
	case http.MethodGet:
		b.getTopic(w, r, topicName)
	case http.MethodDelete:
		b.deleteTopic(w, r, topicName)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getTopic handles GET /api/v1/topics/:name
func (b *Broker) getTopic(w http.ResponseWriter, r *http.Request, topicName string) {
	if b.metaStore == nil {
		http.Error(w, "Metadata store not available", http.StatusServiceUnavailable)
		return
	}

	topic, exists := b.metaStore.GetTopic(topicName)
	if !exists {
		http.Error(w, "Topic not found", http.StatusNotFound)
		return
	}

	resp := TopicResponse{
		Name:              topic.Name,
		NumPartitions:     topic.NumPartitions,
		ReplicationFactor: topic.ReplicationFactor,
		Partitions:        b.buildPartitionInfos(topicName, topic.NumPartitions),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// deleteTopic handles DELETE /api/v1/topics/:name
func (b *Broker) deleteTopic(w http.ResponseWriter, r *http.Request, topicName string) {
	if b.metaStore == nil {
		http.Error(w, "Metadata store not available", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := b.metaStore.DeleteTopic(ctx, topicName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete topic: %v", err), http.StatusInternalServerError)
		return
	}

	b.logger.Info("Topic deleted via API", logging.Fields{
		"topic": topicName,
	})

	w.WriteHeader(http.StatusNoContent)
}

// handleTopicPartitions handles GET /api/v1/topics/:name/partitions
func (b *Broker) handleTopicPartitions(w http.ResponseWriter, r *http.Request, topicName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.metaStore == nil {
		http.Error(w, "Metadata store not available", http.StatusServiceUnavailable)
		return
	}

	topic, exists := b.metaStore.GetTopic(topicName)
	if !exists {
		http.Error(w, "Topic not found", http.StatusNotFound)
		return
	}

	partitions := b.buildPartitionInfos(topicName, topic.NumPartitions)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(partitions)
}

// buildPartitionInfos builds per-partition detail for a topic: leader,
// replicas, and ISR from cluster metadata, plus real beginning/end offsets
// and message counts read from local storage via topicManager.PartitionOffsets.
// This is the one place both getTopic and handleTopicPartitions get partition
// detail from, so neither can drift back to reporting fabricated zero
// offsets independently of the other.
//
// Offsets come from local storage, so they are only populated for partitions
// this broker actually hosts (or when topicManager isn't wired up at all) -
// a partition led elsewhere keeps its zero values, which is a real "unknown
// here", not a fabricated one.
func (b *Broker) buildPartitionInfos(topicName string, numPartitions int) []PartitionInfo {
	partitions := make([]PartitionInfo, numPartitions)
	for i := 0; i < numPartitions; i++ {
		partInfo, exists := b.metaStore.GetPartition(topicName, i)
		if !exists {
			continue
		}

		partitions[i] = PartitionInfo{
			ID:       int32(i),
			Leader:   int32(partInfo.Leader),
			Replicas: convertUint64SliceToInt32(partInfo.Replicas),
			ISR:      convertUint64SliceToInt32(partInfo.ISR),
		}

		if b.topicManager == nil {
			continue
		}
		//nolint:gosec // partition IDs are bounded by numPartitions
		start, end, _, err := b.topicManager.PartitionOffsets(topicName, uint32(i))
		if err != nil {
			continue
		}
		partitions[i].BeginningOffset = start
		partitions[i].EndOffset = end
		partitions[i].MessageCount = end - start
	}
	return partitions
}

// MessageInfo represents message metadata for browsing
type MessageInfo struct {
	Offset  int64             `json:"offset"`
	Key     string            `json:"key,omitempty"`
	Value   string            `json:"value"`
	Headers map[string]string `json:"headers,omitempty"`
	// Timestamp is the message timestamp in Unix nanoseconds.
	Timestamp int64 `json:"timestamp"`
	// Encoding is set to "base64" when any of Key, Value or Headers held
	// non-UTF-8 bytes and had to be base64-encoded to survive JSON. It is
	// omitted when everything was plain text.
	Encoding string `json:"encoding,omitempty"`
}

// Bounds for the messages endpoint's limit parameter.
const (
	defaultMessageLimit = 100
	maxMessageLimit     = 1000
)

// handleTopicMessages handles GET /api/v1/topics/:name/messages.
//
// Query parameters:
//   - partition: partition to read from (default 0)
//   - offset:    offset to start reading at (default 0, clamped up to the
//     partition's log start offset)
//   - limit:     maximum messages to return (default 100, capped at 1000)
//
// Message keys and values are returned as strings. Non-UTF-8 payloads are
// base64-encoded and flagged via the "encoding" field so binary data survives
// the round-trip through JSON intact.
func (b *Broker) handleTopicMessages(w http.ResponseWriter, r *http.Request, topicName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.topicManager == nil {
		http.Error(w, "Storage not initialized", http.StatusServiceUnavailable)
		return
	}

	partitionID, offset, limit, err := parseMessageQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stored, err := b.topicManager.ReadMessages(topicName, partitionID, offset, limit)
	if err != nil {
		// GetPartition reports both a missing topic and a missing partition as
		// an error; either way the client asked for something that isn't there.
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	messages := make([]MessageInfo, 0, len(stored))
	for _, msg := range stored {
		messages = append(messages, toMessageInfo(msg))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

// parseMessageQuery extracts and validates the messages endpoint's query
// parameters. Absent parameters take their defaults; present but malformed
// ones are rejected rather than silently defaulted, so a typo does not look
// like an empty partition.
func parseMessageQuery(r *http.Request) (partitionID uint32, offset int64, limit int, err error) {
	query := r.URL.Query()
	limit = defaultMessageLimit

	if raw := query.Get("partition"); raw != "" {
		parsed, parseErr := strconv.ParseUint(raw, 10, 32)
		if parseErr != nil {
			return 0, 0, 0, fmt.Errorf("invalid partition %q: must be a non-negative integer", raw)
		}
		partitionID = uint32(parsed)
	}

	if raw := query.Get("offset"); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed < 0 {
			return 0, 0, 0, fmt.Errorf("invalid offset %q: must be a non-negative integer", raw)
		}
		offset = parsed
	}

	if raw := query.Get("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			return 0, 0, 0, fmt.Errorf("invalid limit %q: must be a non-negative integer", raw)
		}
		limit = parsed
	}

	if limit > maxMessageLimit {
		limit = maxMessageLimit
	}

	return partitionID, offset, limit, nil
}

// toMessageInfo converts a stored message into its JSON representation.
func toMessageInfo(msg *storage.Message) MessageInfo {
	info := MessageInfo{
		Offset:    int64(msg.Offset),
		Timestamp: msg.Timestamp.UnixNano(),
	}

	key, keyBinary := encodePayload(msg.Key)
	value, valueBinary := encodePayload(msg.Value)
	info.Key = key
	info.Value = value

	if keyBinary || valueBinary {
		info.Encoding = "base64"
	}

	if len(msg.Headers) > 0 {
		info.Headers = make(map[string]string, len(msg.Headers))
		for name, raw := range msg.Headers {
			headerValue, headerBinary := encodePayload(raw)
			info.Headers[name] = headerValue
			if headerBinary {
				info.Encoding = "base64"
			}
		}
	}

	return info
}

// encodePayload renders a byte payload for JSON, falling back to base64 when
// it is not valid UTF-8. The bool reports whether base64 was used.
func encodePayload(data []byte) (string, bool) {
	if len(data) == 0 {
		return "", false
	}
	if utf8.Valid(data) {
		return string(data), false
	}
	return base64.StdEncoding.EncodeToString(data), true
}

// ==================== Consumer Group Endpoints ====================

// ConsumerGroupInfo represents consumer group metadata
type ConsumerGroupInfo struct {
	GroupID     string       `json:"group_id"`
	State       string       `json:"state"`
	Protocol    string       `json:"protocol"`
	Members     []MemberInfo `json:"members"`
	Coordinator int32        `json:"coordinator"`
	TotalLag    int64        `json:"total_lag"`
}

// MemberInfo represents consumer group member metadata
type MemberInfo struct {
	MemberID   string  `json:"member_id"`
	ClientID   string  `json:"client_id"`
	ClientHost string  `json:"client_host"`
	Partitions []int32 `json:"partitions"`
	JoinedAt   int64   `json:"joined_at"`
}

// handleConsumerGroups handles GET /api/v1/consumer-groups
func (b *Broker) handleConsumerGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.groupCoordinator == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ConsumerGroupInfo{})
		return
	}

	groups := b.groupCoordinator.ListGroups()
	groupInfos := make([]ConsumerGroupInfo, 0, len(groups))

	for _, group := range groups {
		totalLag := int64(0) // TODO: Calculate actual lag

		members := make([]MemberInfo, 0, len(group.Members))
		for _, member := range group.Members {
			partitions := []int32{}
			if member.Assignment != nil {
				for _, parts := range member.Assignment.Partitions {
					partitions = append(partitions, parts...)
				}
			}

			memberInfo := MemberInfo{
				MemberID:   member.MemberID,
				ClientID:   member.ClientID,
				ClientHost: member.ClientHost,
				Partitions: partitions,
				JoinedAt:   member.JoinTime.Unix(),
			}
			members = append(members, memberInfo)
		}

		info := ConsumerGroupInfo{
			GroupID:     group.GroupID,
			State:       string(group.State),
			Protocol:    group.ProtocolName,
			Members:     members,
			Coordinator: b.config.BrokerID,
			TotalLag:    totalLag,
		}
		groupInfos = append(groupInfos, info)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(groupInfos)
}

// handleConsumerGroupOperations handles consumer group-specific operations
func (b *Broker) handleConsumerGroupOperations(w http.ResponseWriter, r *http.Request) {
	// Extract group ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/consumer-groups/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Group ID required", http.StatusBadRequest)
		return
	}

	groupID := parts[0]

	// Check for sub-resources
	if len(parts) > 1 && parts[1] == "lag" {
		b.handleConsumerGroupLag(w, r, groupID)
		return
	}

	// Handle group operations
	switch r.Method {
	case http.MethodGet:
		b.getConsumerGroup(w, r, groupID)
	case http.MethodDelete:
		b.deleteConsumerGroup(w, r, groupID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// deleteConsumerGroup handles DELETE /api/v1/consumer-groups/:id.
//
// An unknown group is a plain 404. A group that still has active members is
// rejected with 409 Conflict (mirroring Kafka's NON_EMPTY_GROUP) rather than
// deleted out from under the consumers still relying on its offsets.
func (b *Broker) deleteConsumerGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if b.groupCoordinator == nil {
		http.Error(w, "Group coordinator not available", http.StatusServiceUnavailable)
		return
	}

	err := b.groupCoordinator.DeleteGroup(groupID)
	if err == nil {
		b.logger.Info("Consumer group deleted via API", logging.Fields{"group_id": groupID})
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var notEmpty *group.GroupNotEmptyError
	switch {
	case errors.Is(err, group.ErrGroupNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.As(err, &notEmpty):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// getConsumerGroup handles GET /api/v1/consumer-groups/:id
func (b *Broker) getConsumerGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if b.groupCoordinator == nil {
		http.Error(w, "Group coordinator not available", http.StatusServiceUnavailable)
		return
	}

	group := b.groupCoordinator.GetGroup(groupID)
	if group == nil {
		http.Error(w, "Consumer group not found", http.StatusNotFound)
		return
	}

	members := make([]MemberInfo, 0, len(group.Members))
	for _, member := range group.Members {
		partitions := []int32{}
		if member.Assignment != nil {
			for _, parts := range member.Assignment.Partitions {
				partitions = append(partitions, parts...)
			}
		}

		memberInfo := MemberInfo{
			MemberID:   member.MemberID,
			ClientID:   member.ClientID,
			ClientHost: member.ClientHost,
			Partitions: partitions,
			JoinedAt:   member.JoinTime.Unix(),
		}
		members = append(members, memberInfo)
	}

	info := ConsumerGroupInfo{
		GroupID:     group.GroupID,
		State:       string(group.State),
		Protocol:    group.ProtocolName,
		Members:     members,
		Coordinator: b.config.BrokerID,
		TotalLag:    0, // TODO: Calculate
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// LagInfo represents lag information for a partition
type LagInfo struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	MemberID  string `json:"member_id"`
	Offset    int64  `json:"offset"`
	EndOffset int64  `json:"end_offset"`
	Lag       int64  `json:"lag"`
}

// handleConsumerGroupLag handles GET /api/v1/consumer-groups/:id/lag
func (b *Broker) handleConsumerGroupLag(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.groupCoordinator == nil {
		http.Error(w, "Group coordinator not available", http.StatusServiceUnavailable)
		return
	}

	group := b.groupCoordinator.GetGroup(groupID)
	if group == nil {
		http.Error(w, "Consumer group not found", http.StatusNotFound)
		return
	}

	lagInfos := []LagInfo{}

	// Get committed offsets for the group
	for _, member := range group.Members {
		if member.Assignment == nil {
			continue
		}

		for topic, partitions := range member.Assignment.Partitions {
			for _, partition := range partitions {
				// Get committed offset
				committed, _ := b.groupCoordinator.GetCommittedOffset(groupID, topic, partition)

				// TODO: Get end offset from storage
				endOffset := int64(0)

				lag := endOffset - committed
				if lag < 0 {
					lag = 0
				}

				lagInfo := LagInfo{
					Topic:     topic,
					Partition: partition,
					MemberID:  member.MemberID,
					Offset:    committed,
					EndOffset: endOffset,
					Lag:       lag,
				}
				lagInfos = append(lagInfos, lagInfo)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(lagInfos)
}

// convertUint64SliceToInt32 converts []uint64 to []int32
func convertUint64SliceToInt32(input []uint64) []int32 {
	result := make([]int32, len(input))
	for i, v := range input {
		result[i] = int32(v)
	}
	return result
}

// ==================== Replication Endpoints ====================

// handleReplicationLinks handles GET /api/v1/replication/links and POST /api/v1/replication/links
func (b *Broker) handleReplicationLinks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.listReplicationLinks(w, r)
	case http.MethodPost:
		b.createReplicationLink(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleReplicationLinkOperations handles operations on specific replication links
func (b *Broker) handleReplicationLinkOperations(w http.ResponseWriter, r *http.Request) {
	// Extract link ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/replication/links/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Link ID required", http.StatusBadRequest)
		return
	}

	linkID := parts[0]

	// Handle sub-paths
	if len(parts) > 1 {
		switch parts[1] {
		case "start":
			b.startReplicationLink(w, r, linkID)
		case "stop":
			b.stopReplicationLink(w, r, linkID)
		case "pause":
			b.pauseReplicationLink(w, r, linkID)
		case "resume":
			b.resumeReplicationLink(w, r, linkID)
		case "metrics":
			b.getReplicationLinkMetrics(w, r, linkID)
		case "health":
			b.getReplicationLinkHealth(w, r, linkID)
		case "failover":
			b.failoverReplicationLink(w, r, linkID)
		default:
			http.Error(w, "Unknown operation", http.StatusNotFound)
		}
		return
	}

	// Handle operations on the link itself
	switch r.Method {
	case http.MethodGet:
		b.getReplicationLink(w, r, linkID)
	case http.MethodPut:
		b.updateReplicationLink(w, r, linkID)
	case http.MethodDelete:
		b.deleteReplicationLink(w, r, linkID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ReplicationLinkRequest is the JSON body accepted when creating or updating
// a replication link.
type ReplicationLinkRequest struct {
	ID     string         `json:"id,omitempty"`
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Source ClusterRequest `json:"source_cluster"`
	Target ClusterRequest `json:"target_cluster"`
	Topics []string       `json:"topics,omitempty"`
	// TopicPrefix is prepended to replicated topic names on the target.
	TopicPrefix string `json:"topic_prefix,omitempty"`
}

// ClusterRequest describes one side of a replication link.
type ClusterRequest struct {
	ClusterID        string   `json:"cluster_id"`
	Brokers          []string `json:"brokers,omitempty"`
	BootstrapServers string   `json:"bootstrap_servers,omitempty"`
	// ConnectionTimeoutMs and RequestTimeoutMs default to the package
	// defaults when omitted or zero.
	ConnectionTimeoutMs int `json:"connection_timeout_ms,omitempty"`
	RequestTimeoutMs    int `json:"request_timeout_ms,omitempty"`
	MaxRetries          int `json:"max_retries,omitempty"`
}

// ReplicationLinkInfo is the JSON representation of a replication link.
type ReplicationLinkInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Status        string   `json:"status"`
	SourceCluster string   `json:"source_cluster"`
	TargetCluster string   `json:"target_cluster"`
	Topics        []string `json:"topics,omitempty"`
	TopicPrefix   string   `json:"topic_prefix,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	StartedAt     string   `json:"started_at,omitempty"`
	StoppedAt     string   `json:"stopped_at,omitempty"`
}

// toReplicationLinkInfo converts an internal link into its JSON form.
func toReplicationLinkInfo(l *link.ReplicationLink) ReplicationLinkInfo {
	info := ReplicationLinkInfo{
		ID:            l.ID,
		Name:          l.Name,
		Type:          string(l.Type),
		Status:        string(l.Status),
		SourceCluster: l.SourceCluster.ClusterID,
		TargetCluster: l.TargetCluster.ClusterID,
		Topics:        l.Topics,
		TopicPrefix:   l.TopicPrefix,
		CreatedAt:     l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     l.UpdatedAt.Format(time.RFC3339),
	}
	if !l.StartedAt.IsZero() {
		info.StartedAt = l.StartedAt.Format(time.RFC3339)
	}
	if !l.StoppedAt.IsZero() {
		info.StoppedAt = l.StoppedAt.Format(time.RFC3339)
	}
	return info
}

// toClusterConfig converts a cluster request into a link.ClusterConfig,
// filling unset timeouts from the package defaults so a minimal request body
// still validates.
func toClusterConfig(req ClusterRequest) link.ClusterConfig {
	config := link.DefaultClusterConfig()
	config.ClusterID = req.ClusterID
	config.Brokers = req.Brokers
	config.BootstrapServers = req.BootstrapServers

	if req.ConnectionTimeoutMs > 0 {
		config.ConnectionTimeout = time.Duration(req.ConnectionTimeoutMs) * time.Millisecond
	}
	if req.RequestTimeoutMs > 0 {
		config.RequestTimeout = time.Duration(req.RequestTimeoutMs) * time.Millisecond
	}
	if req.MaxRetries > 0 {
		config.MaxRetries = req.MaxRetries
	}

	return config
}

// toReplicationLink converts a request body into an internal link definition.
func (req *ReplicationLinkRequest) toReplicationLink() *link.ReplicationLink {
	linkType := link.ReplicationType(req.Type)
	if req.Type == "" {
		linkType = link.ReplicationTypeActivePassive
	}

	return &link.ReplicationLink{
		ID:             req.ID,
		Name:           req.Name,
		Type:           linkType,
		SourceCluster:  toClusterConfig(req.Source),
		TargetCluster:  toClusterConfig(req.Target),
		Topics:         req.Topics,
		TopicPrefix:    req.TopicPrefix,
		Config:         link.DefaultReplicationConfig(),
		FailoverConfig: link.DefaultFailoverConfig(),
	}
}

// replicationManagerReady reports whether the replication manager is wired up,
// writing a 503 when it is not.
func (b *Broker) replicationManagerReady(w http.ResponseWriter) bool {
	if b.replicationManager == nil {
		http.Error(w, "Replication manager not available", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// writeReplicationError maps a replication manager error onto an HTTP status:
// an unknown link is 404, anything else is 400 (the caller's request was
// rejected) - the manager does not return transport-level failures.
func writeReplicationError(w http.ResponseWriter, err error) {
	if errors.Is(err, link.ErrLinkNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// decodeReplicationLinkRequest reads and validates a link request body.
func decodeReplicationLinkRequest(w http.ResponseWriter, r *http.Request) (*ReplicationLinkRequest, bool) {
	var req ReplicationLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return nil, false
	}
	return &req, true
}

// listReplicationLinks returns all replication links.
func (b *Broker) listReplicationLinks(w http.ResponseWriter, r *http.Request) {
	if !b.replicationManagerReady(w) {
		return
	}

	links, err := b.replicationManager.ListLinks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	infos := make([]ReplicationLinkInfo, 0, len(links))
	for _, l := range links {
		infos = append(infos, toReplicationLinkInfo(l))
	}

	// Sort by ID so repeated calls return a stable order.
	sort.Slice(infos, func(i, j int) bool { return infos[i].ID < infos[j].ID })

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infos)
}

// createReplicationLink creates a new replication link
func (b *Broker) createReplicationLink(w http.ResponseWriter, r *http.Request) {
	if !b.replicationManagerReady(w) {
		return
	}

	req, ok := decodeReplicationLinkRequest(w, r)
	if !ok {
		return
	}

	newLink := req.toReplicationLink()
	if err := b.replicationManager.CreateLink(newLink); err != nil {
		writeReplicationError(w, err)
		return
	}

	b.logger.Info("Replication link created via API", logging.Fields{
		"link_id": newLink.ID,
		"name":    newLink.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toReplicationLinkInfo(newLink))
}

// getReplicationLink returns details of a specific replication link
func (b *Broker) getReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	if !b.replicationManagerReady(w) {
		return
	}

	l, err := b.replicationManager.GetLink(linkID)
	if err != nil {
		writeReplicationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toReplicationLinkInfo(l))
}

// updateReplicationLink updates a replication link configuration
func (b *Broker) updateReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	if !b.replicationManagerReady(w) {
		return
	}

	req, ok := decodeReplicationLinkRequest(w, r)
	if !ok {
		return
	}

	// The path is authoritative for identity; a body that names a different
	// ID would otherwise silently update (or fail on) the wrong link.
	req.ID = linkID

	if err := b.replicationManager.UpdateLink(linkID, req.toReplicationLink()); err != nil {
		writeReplicationError(w, err)
		return
	}

	updated, err := b.replicationManager.GetLink(linkID)
	if err != nil {
		writeReplicationError(w, err)
		return
	}

	b.logger.Info("Replication link updated via API", logging.Fields{"link_id": linkID})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toReplicationLinkInfo(updated))
}

// deleteReplicationLink deletes a replication link
func (b *Broker) deleteReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	if !b.replicationManagerReady(w) {
		return
	}

	if err := b.replicationManager.DeleteLink(linkID); err != nil {
		writeReplicationError(w, err)
		return
	}

	b.logger.Info("Replication link deleted via API", logging.Fields{"link_id": linkID})

	w.WriteHeader(http.StatusNoContent)
}

// replicationLinkAction runs a state-changing operation on a link and responds
// with the link's resulting state. Every such endpoint is POST-only.
func (b *Broker) replicationLinkAction(
	w http.ResponseWriter,
	r *http.Request,
	linkID string,
	action string,
	run func(string) error,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !b.replicationManagerReady(w) {
		return
	}

	if err := run(linkID); err != nil {
		writeReplicationError(w, err)
		return
	}

	l, err := b.replicationManager.GetLink(linkID)
	if err != nil {
		writeReplicationError(w, err)
		return
	}

	b.logger.Info("Replication link action via API", logging.Fields{
		"link_id": linkID,
		"action":  action,
		"status":  string(l.Status),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toReplicationLinkInfo(l))
}

// startReplicationLink starts a replication link
func (b *Broker) startReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	b.replicationLinkAction(w, r, linkID, "start", func(id string) error {
		return b.replicationManager.StartLink(id)
	})
}

// stopReplicationLink stops a replication link
func (b *Broker) stopReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	b.replicationLinkAction(w, r, linkID, "stop", func(id string) error {
		return b.replicationManager.StopLink(id)
	})
}

// pauseReplicationLink pauses a replication link
func (b *Broker) pauseReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	b.replicationLinkAction(w, r, linkID, "pause", func(id string) error {
		return b.replicationManager.PauseLink(id)
	})
}

// resumeReplicationLink resumes a paused replication link
func (b *Broker) resumeReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	b.replicationLinkAction(w, r, linkID, "resume", func(id string) error {
		return b.replicationManager.ResumeLink(id)
	})
}

// getReplicationLinkMetrics returns metrics for a replication link
func (b *Broker) getReplicationLinkMetrics(w http.ResponseWriter, r *http.Request, linkID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !b.replicationManagerReady(w) {
		return
	}

	metrics, err := b.replicationManager.GetMetrics(linkID)
	if err != nil {
		writeReplicationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

// getReplicationLinkHealth returns health status for a replication link
func (b *Broker) getReplicationLinkHealth(w http.ResponseWriter, r *http.Request, linkID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !b.replicationManagerReady(w) {
		return
	}

	health, err := b.replicationManager.GetHealth(linkID)
	if err != nil {
		writeReplicationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(health)
}

// failoverReplicationLink triggers a manual failover
func (b *Broker) failoverReplicationLink(w http.ResponseWriter, r *http.Request, linkID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !b.replicationManagerReady(w) {
		return
	}

	event, err := b.replicationManager.Failover(linkID)
	if err != nil {
		writeReplicationError(w, err)
		return
	}

	b.logger.Info("Replication link failover triggered via API", logging.Fields{
		"link_id": linkID,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(event)
}

// ==================== Security Endpoints ====================

// handleSecurityStatus handles GET /api/v1/security/status
func (b *Broker) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.securityManager == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":        false,
			"authentication": false,
			"authorization":  false,
			"audit":          false,
			"encryption":     false,
		})
		return
	}

	status := map[string]interface{}{
		"enabled":        true,
		"authentication": b.securityManager.IsAuthenticationEnabled(),
		"authorization":  b.securityManager.IsAuthorizationEnabled(),
		"audit":          b.securityManager.IsAuditEnabled(),
		"encryption":     b.securityManager.IsEncryptionEnabled(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleSecurityACLs handles GET /api/v1/security/acls (list) and POST (create)
func (b *Broker) handleSecurityACLs(w http.ResponseWriter, r *http.Request) {
	if b.securityManager == nil {
		http.Error(w, "Security not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		acls := b.securityManager.ListACLs()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"acls":  acls,
			"count": len(acls),
		})

	case http.MethodPost:
		var req struct {
			Principal    string `json:"principal"`
			ResourceType string `json:"resource_type"`
			ResourceName string `json:"resource_name"`
			PatternType  string `json:"pattern_type"`
			Action       string `json:"action"`
			Permission   string `json:"permission"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Create ACL entry
		acl := &security.ACLEntry{
			ID:           fmt.Sprintf("acl-%d", time.Now().UnixNano()),
			Principal:    req.Principal,
			ResourceType: security.ResourceType(req.ResourceType),
			ResourceName: req.ResourceName,
			PatternType:  security.PatternType(req.PatternType),
			Action:       security.Action(req.Action),
			Permission:   security.Permission(req.Permission),
			CreatedAt:    time.Now(),
			CreatedBy:    "admin", // TODO: Get from authenticated principal
		}

		if err := b.securityManager.AddACL(acl); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(acl)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSecurityACLOperations handles DELETE /api/v1/security/acls/:id
func (b *Broker) handleSecurityACLOperations(w http.ResponseWriter, r *http.Request) {
	if b.securityManager == nil {
		http.Error(w, "Security not enabled", http.StatusServiceUnavailable)
		return
	}

	aclID := strings.TrimPrefix(r.URL.Path, "/api/v1/security/acls/")

	switch r.Method {
	case http.MethodDelete:
		if err := b.securityManager.RemoveACL(aclID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSecurityUsers handles GET /api/v1/security/users (list) and POST (create)
func (b *Broker) handleSecurityUsers(w http.ResponseWriter, r *http.Request) {
	if b.securityManager == nil {
		http.Error(w, "Security not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// TODO: Implement user listing when security manager supports it
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []string{},
			"count": 0,
		})

	case http.MethodPost:
		var req struct {
			Username string   `json:"username"`
			Password string   `json:"password"`
			Method   string   `json:"auth_method"`
			Groups   []string `json:"groups"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Add user through security manager
		authMethod := security.AuthMethod(req.Method)
		if err := b.securityManager.AddUser(req.Username, req.Password, authMethod, req.Groups); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"username": req.Username,
			"status":   "created",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSecurityUserOperations handles DELETE /api/v1/security/users/:username
func (b *Broker) handleSecurityUserOperations(w http.ResponseWriter, r *http.Request) {
	if b.securityManager == nil {
		http.Error(w, "Security not enabled", http.StatusServiceUnavailable)
		return
	}

	_ = strings.TrimPrefix(r.URL.Path, "/api/v1/security/users/")

	switch r.Method {
	case http.MethodDelete:
		// TODO: Implement user deletion when security manager supports it
		http.Error(w, "Not implemented", http.StatusNotImplemented)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
