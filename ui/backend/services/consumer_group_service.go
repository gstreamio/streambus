package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ConsumerGroupInfo represents consumer group information
type ConsumerGroupInfo struct {
	GroupID     string         `json:"group_id"`
	State       string         `json:"state"`
	Protocol    string         `json:"protocol"`
	Members     []MemberInfo   `json:"members"`
	Coordinator int32          `json:"coordinator"`
	TotalLag    int64          `json:"total_lag"`
}

// MemberInfo represents consumer group member information
type MemberInfo struct {
	MemberID   string  `json:"member_id"`
	ClientID   string  `json:"client_id"`
	ClientHost string  `json:"client_host"`
	Partitions []int32 `json:"partitions"`
	JoinedAt   int64   `json:"joined_at"`
}

// PartitionAssignment represents partition assignment with lag
type PartitionAssignment struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	MemberID  string `json:"member_id"`
	Offset    int64  `json:"offset"`
	EndOffset int64  `json:"end_offset"`
	Lag       int64  `json:"lag"`
}

// ConsumerGroupService handles consumer group operations
type ConsumerGroupService struct {
	brokerService *BrokerService
	httpClient    *http.Client
}

// NewConsumerGroupService creates a new consumer group service
func NewConsumerGroupService(brokerService *BrokerService) *ConsumerGroupService {
	return &ConsumerGroupService{
		brokerService: brokerService,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ListGroups returns all consumer groups
func (s *ConsumerGroupService) ListGroups(ctx context.Context) ([]ConsumerGroupInfo, error) {
	url := fmt.Sprintf("http://%s/api/v1/consumer-groups", s.brokerService.brokers[0])

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

	var groups []ConsumerGroupInfo
	if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return groups, nil
}

// GetGroup returns a specific consumer group
func (s *ConsumerGroupService) GetGroup(ctx context.Context, groupID string) (*ConsumerGroupInfo, error) {
	url := fmt.Sprintf("http://%s/api/v1/consumer-groups/%s", s.brokerService.brokers[0], groupID)

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

	var group ConsumerGroupInfo
	if err := json.NewDecoder(resp.Body).Decode(&group); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &group, nil
}

// GetGroupLag returns lag information for a consumer group
func (s *ConsumerGroupService) GetGroupLag(ctx context.Context, groupID string) ([]PartitionAssignment, error) {
	url := fmt.Sprintf("http://%s/api/v1/consumer-groups/%s/lag", s.brokerService.brokers[0], groupID)

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

	var assignments []PartitionAssignment
	if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return assignments, nil
}

// ResetOffset resets consumer group offset
func (s *ConsumerGroupService) ResetOffset(ctx context.Context, groupID string, topic string, partition int, offset int64) error {
	// TODO: Implement when broker API supports offset reset
	return fmt.Errorf("offset reset not yet implemented")
}
