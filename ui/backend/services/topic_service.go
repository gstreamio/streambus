package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TopicInfo represents topic information
type TopicInfo struct {
	Name              string            `json:"name"`
	NumPartitions     int               `json:"num_partitions"`
	ReplicationFactor int               `json:"replication_factor"`
	Partitions        []PartitionInfo   `json:"partitions,omitempty"`
}

// PartitionInfo represents partition information
type PartitionInfo struct {
	ID              int32   `json:"id"`
	Leader          int32   `json:"leader"`
	Replicas        []int32 `json:"replicas"`
	ISR             []int32 `json:"isr"` // In-Sync Replicas
	BeginningOffset int64   `json:"beginning_offset"`
	EndOffset       int64   `json:"end_offset"`
	MessageCount    int64   `json:"message_count"`
}

// Message represents a message
type Message struct {
	Offset    int64             `json:"offset"`
	Key       string            `json:"key"`
	Value     string            `json:"value"`
	Timestamp int64             `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// TopicService handles topic operations
type TopicService struct {
	brokerService *BrokerService
	httpClient    *http.Client
}

// NewTopicService creates a new topic service
func NewTopicService(brokerService *BrokerService) *TopicService {
	return &TopicService{
		brokerService: brokerService,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ListTopics returns all topics
func (s *TopicService) ListTopics(ctx context.Context) ([]TopicInfo, error) {
	url := fmt.Sprintf("http://%s/api/v1/topics", s.brokerService.brokers[0])

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call broker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("broker API returned %d: %s", resp.StatusCode, string(body))
	}

	var topics []TopicInfo
	if err := json.NewDecoder(resp.Body).Decode(&topics); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return topics, nil
}

// GetTopic returns topic details
func (s *TopicService) GetTopic(ctx context.Context, name string) (*TopicInfo, error) {
	url := fmt.Sprintf("http://%s/api/v1/topics/%s", s.brokerService.brokers[0], name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call broker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("broker API returned %d: %s", resp.StatusCode, string(body))
	}

	var topic TopicInfo
	if err := json.NewDecoder(resp.Body).Decode(&topic); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &topic, nil
}

// CreateTopic creates a new topic
func (s *TopicService) CreateTopic(ctx context.Context, name string, numPartitions, replicationFactor int, config map[string]string) error {
	url := fmt.Sprintf("http://%s/api/v1/topics", s.brokerService.brokers[0])

	requestBody := map[string]interface{}{
		"name":               name,
		"num_partitions":     numPartitions,
		"replication_factor": replicationFactor,
		"config":             config,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call broker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("broker API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteTopic deletes a topic
func (s *TopicService) DeleteTopic(ctx context.Context, name string) error {
	url := fmt.Sprintf("http://%s/api/v1/topics/%s", s.brokerService.brokers[0], name)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call broker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("broker API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateTopicConfig updates topic configuration
func (s *TopicService) UpdateTopicConfig(ctx context.Context, name string, config map[string]string) error {
	// TODO: Implement when broker API supports config updates
	return fmt.Errorf("topic config update not yet implemented")
}

// GetPartitions returns partitions for a topic
func (s *TopicService) GetPartitions(ctx context.Context, topicName string) ([]PartitionInfo, error) {
	url := fmt.Sprintf("http://%s/api/v1/topics/%s/partitions", s.brokerService.brokers[0], topicName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call broker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("broker API returned %d: %s", resp.StatusCode, string(body))
	}

	var partitions []PartitionInfo
	if err := json.NewDecoder(resp.Body).Decode(&partitions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return partitions, nil
}

// GetMessages returns messages from a topic
func (s *TopicService) GetMessages(ctx context.Context, topicName string, partition int, offset int64, limit int) ([]Message, error) {
	url := fmt.Sprintf("http://%s/api/v1/topics/%s/messages?partition=%d&offset=%d&limit=%d",
		s.brokerService.brokers[0], topicName, partition, offset, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call broker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("broker API returned %d: %s", resp.StatusCode, string(body))
	}

	var messages []Message
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return messages, nil
}
