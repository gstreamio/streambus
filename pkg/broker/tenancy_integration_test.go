package broker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/consensus"
	"github.com/shawntherrien/streambus/pkg/tenancy"
)

func TestMultiTenancyIntegration(t *testing.T) {
	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create broker configuration
	config := &Config{
		BrokerID:           1,
		Host:               "localhost",
		Port:               19092,
		GRPCPort:           19093,
		HTTPPort:           18081,
		DataDir:            t.TempDir() + "/data",
		RaftDataDir:        t.TempDir() + "/raft",
		EnableMultiTenancy: true,
		LogLevel:           "info",
		RaftPeers: []consensus.Peer{
			{ID: 1, Addr: "localhost:17000"},
		},
	}

	// Create and start broker
	broker, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create broker: %v", err)
	}

	if err := broker.Start(); err != nil {
		t.Fatalf("Failed to start broker: %v", err)
	}
	defer broker.Stop()

	// Wait for broker to be ready
	time.Sleep(2 * time.Second)

	// Test tenant management API
	t.Run("CreateTenant", func(t *testing.T) {
		testCreateTenant(t, config.HTTPPort)
	})

	t.Run("ListTenants", func(t *testing.T) {
		testListTenants(t, config.HTTPPort)
	})

	t.Run("GetTenant", func(t *testing.T) {
		testGetTenant(t, config.HTTPPort)
	})

	t.Run("GetTenantStats", func(t *testing.T) {
		testGetTenantStats(t, config.HTTPPort)
	})

	t.Run("UpdateTenant", func(t *testing.T) {
		testUpdateTenant(t, config.HTTPPort)
	})

	t.Run("DeleteTenant", func(t *testing.T) {
		testDeleteTenant(t, config.HTTPPort)
	})
}

func testCreateTenant(t *testing.T, httpPort int) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tenants", httpPort)

	reqBody := map[string]interface{}{
		"id":   "test-tenant",
		"name": "Test Tenant",
		"quotas": map[string]interface{}{
			"MaxBytesPerSecond":    5 * 1024 * 1024,
			"MaxMessagesPerSecond": 5000,
			"MaxStorageBytes":      50 * 1024 * 1024 * 1024,
			"MaxTopics":            50,
			"MaxPartitions":        500,
			"MaxConnections":       100,
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}
}

func testListTenants(t *testing.T, httpPort int) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tenants", httpPort)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to list tenants: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Tenants []map[string]interface{} `json:"tenants"`
		Count   int                      `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have at least the default tenant and test-tenant
	if result.Count < 2 {
		t.Errorf("Expected at least 2 tenants, got %d", result.Count)
	}
}

func testGetTenant(t *testing.T, httpPort int) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tenants/test-tenant", httpPort)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var tenant map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if tenant["id"] != "test-tenant" {
		t.Errorf("Expected tenant ID 'test-tenant', got '%v'", tenant["id"])
	}
}

func testGetTenantStats(t *testing.T, httpPort int) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tenants/test-tenant/stats", httpPort)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to get tenant stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify stats structure
	if _, ok := stats["Tenant"]; !ok {
		t.Error("Expected 'Tenant' field in stats")
	}
	if _, ok := stats["Usage"]; !ok {
		t.Error("Expected 'Usage' field in stats")
	}
	if _, ok := stats["Utilization"]; !ok {
		t.Error("Expected 'Utilization' field in stats")
	}
}

func testUpdateTenant(t *testing.T, httpPort int) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tenants/test-tenant", httpPort)

	reqBody := map[string]interface{}{
		"name": "Updated Test Tenant",
		"quotas": map[string]interface{}{
			"MaxBytesPerSecond": 10 * 1024 * 1024,
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to update tenant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func testDeleteTenant(t *testing.T, httpPort int) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tenants/test-tenant", httpPort)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete tenant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
}

func TestTenantQuotaEnforcement(t *testing.T) {
	// Create manager with test tenant
	manager := tenancy.NewManager()

	quotas := &tenancy.Quotas{
		MaxBytesPerSecond:    1000,
		MaxMessagesPerSecond: 10,
		MaxConnections:       5,
		MaxTopics:            3,
		MaxPartitions:        20,
	}

	_, err := manager.CreateTenant("quota-test", "Quota Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	t.Run("EnforceThroughputQuota", func(t *testing.T) {
		// Should succeed under quota
		err := manager.EnforceProduceQuota("quota-test", 500, 5)
		if err != nil {
			t.Errorf("Expected success under quota, got error: %v", err)
		}

		// Should fail when exceeding bytes quota
		err = manager.EnforceProduceQuota("quota-test", 600, 3)
		if err == nil {
			t.Error("Expected quota error when exceeding bytes quota")
		}
	})

	t.Run("EnforceConnectionQuota", func(t *testing.T) {
		// Add connections up to limit
		for i := 0; i < 5; i++ {
			err := manager.EnforceConnectionQuota("quota-test")
			if err != nil {
				t.Errorf("Failed to add connection %d: %v", i, err)
			}
		}

		// Should fail when exceeding connection quota
		err := manager.EnforceConnectionQuota("quota-test")
		if err == nil {
			t.Error("Expected quota error when exceeding connection quota")
		}
	})

	t.Run("EnforceTopicQuota", func(t *testing.T) {
		// Add topics up to limit
		for i := 0; i < 3; i++ {
			err := manager.EnforceTopicQuota("quota-test", 5)
			if err != nil {
				t.Errorf("Failed to add topic %d: %v", i, err)
			}
		}

		// Should fail when exceeding topic quota
		err := manager.EnforceTopicQuota("quota-test", 1)
		if err == nil {
			t.Error("Expected quota error when exceeding topic quota")
		}
	})

	t.Run("GetUsageStats", func(t *testing.T) {
		usage, err := manager.GetUsage("quota-test")
		if err != nil {
			t.Fatalf("Failed to get usage: %v", err)
		}

		// 5 successful connections (6th failed)
		if usage.Connections != 5 {
			t.Errorf("Expected 5 connections, got %d", usage.Connections)
		}

		// 3 successful topics (4th failed)
		if usage.Topics != 3 {
			t.Errorf("Expected 3 topics, got %d", usage.Topics)
		}

		// Total partitions should be 15 (3 topics × 5 partitions)
		if usage.Partitions != 15 {
			t.Errorf("Expected 15 partitions, got %d", usage.Partitions)
		}
	})

	t.Run("GetUtilization", func(t *testing.T) {
		utilization, err := manager.GetUtilization("quota-test")
		if err != nil {
			t.Fatalf("Failed to get utilization: %v", err)
		}

		// Connection utilization should be 100% (5/5)
		if utilization["connections"] != 100.0 {
			t.Errorf("Expected connection utilization 100%%, got %.2f%%", utilization["connections"])
		}

		// Topic utilization should be 100% (3/3)
		if utilization["topics"] != 100.0 {
			t.Errorf("Expected topic utilization 100%%, got %.2f%%", utilization["topics"])
		}
	})
}
