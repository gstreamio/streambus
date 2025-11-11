package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// BrokerInfo represents broker information
type BrokerInfo struct {
	ID      int    `json:"id"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Status  string `json:"status"` // "online", "offline", "degraded"
	Leader  bool   `json:"leader"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// ClusterInfo represents cluster information
type ClusterInfo struct {
	ClusterID       string `json:"cluster_id"`
	ControllerID    int    `json:"controller_id"`
	Version         string `json:"version"`
	TotalBrokers    int    `json:"total_brokers"`
	ActiveBrokers   int    `json:"active_brokers"`
	TotalTopics     int    `json:"total_topics"`
	TotalPartitions int    `json:"total_partitions"`
	Uptime          string `json:"uptime"`
}

// BrokerResources represents broker resource usage
type BrokerResources struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskPercent   float64 `json:"disk_percent"`
	DiskUsedGB    int64   `json:"disk_used_gb"`
	DiskTotalGB   int64   `json:"disk_total_gb"`
}

// BrokerDetail represents detailed broker information
type BrokerDetail struct {
	ID        int              `json:"id"`
	Host      string           `json:"host"`
	Port      int              `json:"port"`
	Status    string           `json:"status"`
	Leader    bool             `json:"leader"`
	Version   string           `json:"version"`
	Uptime    string           `json:"uptime"`
	Resources *BrokerResources `json:"resources,omitempty"`
}

// BrokerService handles broker operations
type BrokerService struct {
	brokers    []string
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]interface{}
}

// NewBrokerService creates a new broker service
func NewBrokerService(brokers []string) *BrokerService {
	return &BrokerService{
		brokers: brokers,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]interface{}),
	}
}

// GetClusterInfo returns cluster information
func (s *BrokerService) GetClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	// Call broker admin API
	url := fmt.Sprintf("http://%s/api/v1/cluster", s.brokers[0])

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

	var clusterInfo ClusterInfo
	if err := json.NewDecoder(resp.Body).Decode(&clusterInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &clusterInfo, nil
}

// ListBrokers returns all brokers
func (s *BrokerService) ListBrokers(ctx context.Context) ([]BrokerInfo, error) {
	url := fmt.Sprintf("http://%s/api/v1/brokers", s.brokers[0])

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

	var brokers []BrokerInfo
	if err := json.NewDecoder(resp.Body).Decode(&brokers); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return brokers, nil
}

// GetBroker returns a specific broker
func (s *BrokerService) GetBroker(ctx context.Context, id int) (*BrokerDetail, error) {
	url := fmt.Sprintf("http://%s/api/v1/brokers/%d", s.brokers[0], id)

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

	var broker BrokerDetail
	if err := json.NewDecoder(resp.Body).Decode(&broker); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &broker, nil
}

// GetClusterHealth returns cluster health status
func (s *BrokerService) GetClusterHealth(ctx context.Context) (string, error) {
	info, err := s.GetClusterInfo(ctx)
	if err != nil {
		return "offline", err
	}

	// Determine health based on active brokers
	if info.ActiveBrokers == 0 {
		return "offline", nil
	} else if info.ActiveBrokers < info.TotalBrokers {
		return "degraded", nil
	}

	return "healthy", nil
}

// Ping checks if broker is reachable
func (s *BrokerService) Ping(ctx context.Context, broker string) error {
	url := fmt.Sprintf("http://%s/health/live", broker)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
