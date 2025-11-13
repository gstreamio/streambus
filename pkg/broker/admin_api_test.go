package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gstreamio/streambus/pkg/cluster"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/metadata"
)

// mockClusterMetadataStore implements cluster.MetadataStore for testing
type mockClusterMetadataStore struct{}

func (m *mockClusterMetadataStore) StoreBrokerMetadata(ctx context.Context, broker *cluster.BrokerMetadata) error {
	return nil
}

func (m *mockClusterMetadataStore) GetBrokerMetadata(ctx context.Context, brokerID int32) (*cluster.BrokerMetadata, error) {
	return nil, nil
}

func (m *mockClusterMetadataStore) ListBrokers(ctx context.Context) ([]*cluster.BrokerMetadata, error) {
	return []*cluster.BrokerMetadata{}, nil
}

func (m *mockClusterMetadataStore) DeleteBroker(ctx context.Context, brokerID int32) error {
	return nil
}

// newTestBrokerForAPI creates a minimal broker instance for API testing
func newTestBrokerForAPI(t *testing.T) *Broker {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := logging.New(&logging.Config{
		Level:     logging.LevelError,
		Component: "test",
	})

	// Create mock metadata stores
	clusterMetaStore := &mockClusterMetadataStore{}

	// Create broker registry
	registry := cluster.NewBrokerRegistry(clusterMetaStore)

	broker := &Broker{
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		metaStore: nil, // Set to nil for now - the endpoints handle this
		registry:  registry,
		config: &Config{
			BrokerID: 1,
		},
		status: StatusRunning,
	}

	return broker
}

// TestHandleClusterInfo tests the cluster info endpoint
func TestHandleClusterInfo(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	// Register a broker in the registry
	testBroker := &cluster.BrokerMetadata{
		ID:       broker.config.BrokerID,
		Host:     "localhost",
		Port:     9092,
		Status:   cluster.BrokerStatusAlive,
		Capacity: 100,
	}
	broker.registry.RegisterBroker(context.Background(), testBroker)
	broker.registry.RecordHeartbeat(broker.config.BrokerID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster", nil)
	w := httptest.NewRecorder()

	broker.handleClusterInfo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var info ClusterInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Controller ID will be 0 since metaStore is nil  (returns 0 when not leader)
	if info.ControllerID != 0 {
		t.Errorf("Expected ControllerID 0, got %d", info.ControllerID)
	}

	if info.ActiveBrokers != 1 {
		t.Errorf("Expected 1 active broker, got %d", info.ActiveBrokers)
	}
}

// TestHandleClusterInfo_MethodNotAllowed tests the cluster info endpoint with wrong method
func TestHandleClusterInfo_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster", nil)
	w := httptest.NewRecorder()

	broker.handleClusterInfo(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleBrokerList tests the broker list endpoint
func TestHandleBrokerList(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	// Register test brokers
	for i := int32(1); i <= 3; i++ {
		testBroker := &cluster.BrokerMetadata{
			ID:       i,
			Host:     "localhost",
			Port:     9090 + int(i),
			Status:   cluster.BrokerStatusAlive,
			Capacity: 100,
		}
		broker.registry.RegisterBroker(context.Background(), testBroker)
		broker.registry.RecordHeartbeat(i)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/brokers", nil)
	w := httptest.NewRecorder()

	broker.handleBrokerList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var brokers []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&brokers); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(brokers) != 3 {
		t.Errorf("Expected 3 brokers, got %d", len(brokers))
	}
}

// TestHandleBrokerList_MethodNotAllowed tests broker list with wrong method
func TestHandleBrokerList_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/brokers", nil)
	w := httptest.NewRecorder()

	broker.handleBrokerList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleBrokerDetail tests the broker detail endpoint
func TestHandleBrokerDetail(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	// Register a test broker
	testBroker := &cluster.BrokerMetadata{
		ID:       2,
		Host:     "localhost",
		Port:     9092,
		Status:   cluster.BrokerStatusAlive,
		Capacity: 100,
	}
	broker.registry.RegisterBroker(context.Background(), testBroker)
	broker.registry.RecordHeartbeat(2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/brokers/2", nil)
	w := httptest.NewRecorder()

	broker.handleBrokerDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var brokerDetail map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&brokerDetail); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check that ID is present
	if _, ok := brokerDetail["id"]; !ok {
		t.Error("Expected 'id' field in broker detail")
	}
}

// TestHandleBrokerDetail_NotFound tests broker detail with non-existent broker
func TestHandleBrokerDetail_NotFound(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/brokers/999", nil)
	w := httptest.NewRecorder()

	broker.handleBrokerDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleBrokerDetail_InvalidID tests broker detail with invalid ID
func TestHandleBrokerDetail_InvalidID(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/brokers/invalid", nil)
	w := httptest.NewRecorder()

	broker.handleBrokerDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleBrokerDetail_MethodNotAllowed tests broker detail with wrong method
func TestHandleBrokerDetail_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/brokers/1", nil)
	w := httptest.NewRecorder()

	broker.handleBrokerDetail(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// mockConsensusNode implements metadata.ConsensusNode for testing
type mockConsensusNode struct {
	fsm      *metadata.FSM
	isLeader bool
}

func newMockConsensusNode(fsm *metadata.FSM) *mockConsensusNode {
	return &mockConsensusNode{
		fsm:      fsm,
		isLeader: true,
	}
}

func (m *mockConsensusNode) Propose(ctx context.Context, data []byte) error {
	// Directly apply to FSM (simulating immediate consensus)
	return m.fsm.Apply(data)
}

func (m *mockConsensusNode) IsLeader() bool {
	return m.isLeader
}

func (m *mockConsensusNode) Leader() uint64 {
	if m.isLeader {
		return 1
	}
	return 0
}

// newTestBrokerWithMetaStore creates a broker with a mock metadata store
func newTestBrokerWithMetaStore(t *testing.T) (*Broker, *metadata.Store) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := logging.New(&logging.Config{
		Level:     logging.LevelError,
		Component: "test",
	})

	clusterMetaStore := &mockClusterMetadataStore{}
	registry := cluster.NewBrokerRegistry(clusterMetaStore)

	// Create FSM and mock consensus for metadata store
	fsm := metadata.NewFSM()
	consensus := newMockConsensusNode(fsm)
	metaStore := metadata.NewStore(fsm, consensus)

	broker := &Broker{
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		metaStore: metaStore,
		registry:  registry,
		config: &Config{
			BrokerID: 1,
		},
		status: StatusRunning,
	}

	return broker, metaStore
}

// TestHandleTopics_List tests listing topics
func TestHandleTopics_List(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create some test topics
	ctx := context.Background()
	metaStore.CreateTopic(ctx, "topic1", 3, 1, metadata.DefaultTopicConfig())
	metaStore.CreateTopic(ctx, "topic2", 2, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var topics []TopicResponse
	if err := json.NewDecoder(w.Body).Decode(&topics); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(topics) != 2 {
		t.Errorf("Expected 2 topics, got %d", len(topics))
	}
}

// TestHandleTopics_Create tests creating a topic
func TestHandleTopics_Create(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	reqBody := `{"name":"test-topic","num_partitions":3,"replication_factor":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp["name"] != "test-topic" {
		t.Errorf("Expected name 'test-topic', got %v", resp["name"])
	}
}

// TestHandleTopics_Create_InvalidJSON tests creating a topic with invalid JSON
func TestHandleTopics_Create_InvalidJSON(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	reqBody := `{"name":"test-topic","num_partitions":"invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleTopics_Create_EmptyName tests creating a topic without a name
func TestHandleTopics_Create_EmptyName(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	reqBody := `{"name":"","num_partitions":3,"replication_factor":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleTopics_MethodNotAllowed tests topics endpoint with unsupported method
func TestHandleTopics_MethodNotAllowed(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/topics", nil)
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestGetTopic tests getting a specific topic
func TestGetTopic(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic
	ctx := context.Background()
	metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp TopicResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Name != "test-topic" {
		t.Errorf("Expected name 'test-topic', got %s", resp.Name)
	}

	if resp.NumPartitions != 3 {
		t.Errorf("Expected 3 partitions, got %d", resp.NumPartitions)
	}
}

// TestGetTopic_NotFound tests getting a non-existent topic
func TestGetTopic_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/nonexistent", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestDeleteTopic tests deleting a topic
func TestDeleteTopic(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic
	ctx := context.Background()
	metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/topics/test-topic", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// Verify topic was deleted
	if _, exists := metaStore.GetTopic("test-topic"); exists {
		t.Error("Topic should have been deleted")
	}
}

// TestDeleteTopic_NotFound tests deleting a non-existent topic
func TestDeleteTopic_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/topics/nonexistent", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestHandleTopicOperations_MethodNotAllowed tests topic operations with unsupported method
func TestHandleTopicOperations_MethodNotAllowed(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/topics/test-topic", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleTopicPartitions tests getting topic partitions
func TestHandleTopicPartitions(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic with 3 partitions
	ctx := context.Background()
	metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic/partitions", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var partitions []PartitionInfo
	if err := json.NewDecoder(w.Body).Decode(&partitions); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(partitions) != 3 {
		t.Errorf("Expected 3 partitions, got %d", len(partitions))
	}
}

// TestHandleTopicPartitions_NotFound tests getting partitions for non-existent topic
func TestHandleTopicPartitions_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/nonexistent/partitions", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleTopicPartitions_MethodNotAllowed tests partitions endpoint with wrong method
func TestHandleTopicPartitions_MethodNotAllowed(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic
	ctx := context.Background()
	metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/test-topic/partitions", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleTopicMessages_MethodNotAllowed tests messages endpoint with wrong method
func TestHandleTopicMessages_MethodNotAllowed(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic
	ctx := context.Background()
	metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/test-topic/messages", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleConsumerGroups_NilCoordinator tests consumer groups endpoint with nil coordinator
func TestHandleConsumerGroups_NilCoordinator(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/consumer-groups", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroups(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var groups []ConsumerGroupInfo
	if err := json.NewDecoder(w.Body).Decode(&groups); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("Expected 0 groups, got %d", len(groups))
	}
}

// TestHandleConsumerGroups_MethodNotAllowed tests consumer groups endpoint with wrong method
func TestHandleConsumerGroups_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/consumer-groups", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroups(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestGetConsumerGroup_NilCoordinator tests getting consumer group with nil coordinator
func TestGetConsumerGroup_NilCoordinator(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/consumer-groups/test-group", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroupOperations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestGetConsumerGroup_MissingGroupID tests getting consumer group without group ID
func TestGetConsumerGroup_MissingGroupID(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/consumer-groups/", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroupOperations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleConsumerGroupLag_NilCoordinator tests lag endpoint with nil coordinator
func TestHandleConsumerGroupLag_NilCoordinator(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/consumer-groups/test-group/lag", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroupOperations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestHandleConsumerGroupLag_MethodNotAllowed tests lag endpoint with wrong method
func TestHandleConsumerGroupLag_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/consumer-groups/test-group/lag", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroupOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleConsumerGroupOperations_MethodNotAllowed tests group operations with wrong method
func TestHandleConsumerGroupOperations_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/consumer-groups/test-group", nil)
	w := httptest.NewRecorder()

	broker.handleConsumerGroupOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleReplicationLinks_List tests listing replication links
func TestHandleReplicationLinks_List(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/links", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var links []interface{}
	if err := json.NewDecoder(w.Body).Decode(&links); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(links) != 0 {
		t.Errorf("Expected 0 links, got %d", len(links))
	}
}

// TestHandleReplicationLinks_Create tests creating a replication link
func TestHandleReplicationLinks_Create(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinks(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestHandleReplicationLinks_MethodNotAllowed tests replication links with wrong method
func TestHandleReplicationLinks_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/replication/links", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinks(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestGetReplicationLink tests getting a specific replication link
func TestGetReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/links/test-link", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestUpdateReplicationLink tests updating a replication link
func TestUpdateReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/replication/links/test-link", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestDeleteReplicationLink tests deleting a replication link
func TestDeleteReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/replication/links/test-link", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestReplicationLinkOperations_MissingLinkID tests operations without link ID
func TestReplicationLinkOperations_MissingLinkID(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/links/", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestReplicationLinkOperations_MethodNotAllowed tests link operations with wrong method
func TestReplicationLinkOperations_MethodNotAllowed(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestStartReplicationLink tests starting a replication link
func TestStartReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link/start", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestStopReplicationLink tests stopping a replication link
func TestStopReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link/stop", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestPauseReplicationLink tests pausing a replication link
func TestPauseReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link/pause", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestResumeReplicationLink tests resuming a replication link
func TestResumeReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link/resume", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestGetReplicationLinkMetrics tests getting replication link metrics
func TestGetReplicationLinkMetrics(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/links/test-link/metrics", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestGetReplicationLinkHealth tests getting replication link health
func TestGetReplicationLinkHealth(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/links/test-link/health", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestFailoverReplicationLink tests triggering failover on a replication link
func TestFailoverReplicationLink(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link/failover", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("Expected status 501, got %d", w.Code)
	}
}

// TestReplicationLinkOperations_UnknownOperation tests link operations with unknown sub-path
func TestReplicationLinkOperations_UnknownOperation(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/links/test-link/unknown", nil)
	w := httptest.NewRecorder()

	broker.handleReplicationLinkOperations(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
