package server

import (
	"fmt"
	"testing"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/tenancy"
)

func TestNewTenancyHandler(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	if th == nil {
		t.Fatal("Expected TenancyHandler to be created")
	}

	if th.baseHandler != baseHandler {
		t.Error("baseHandler not set correctly")
	}

	if th.tenancyManager != tenancyMgr {
		t.Error("tenancyManager not set correctly")
	}

	if !th.enabled {
		t.Error("enabled should be true")
	}
}

func TestTenancyHandler_Handle_Disabled(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	// Create handler with tenancy disabled
	th := NewTenancyHandler(baseHandler, nil, false)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 123,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: []byte("value1"),
				},
			},
		},
	}

	resp := th.Handle(req)

	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status when tenancy is disabled, got %v", resp.Header.Status)
	}

	stats := th.GetStats()
	if stats["requests_handled"] != 1 {
		t.Errorf("requests_handled = %d, want 1", stats["requests_handled"])
	}
}

func TestTenancyHandler_Handle_NilManager(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	// enabled=true but tenancyManager is nil
	th := NewTenancyHandler(baseHandler, nil, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 456,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: []byte("value1"),
				},
			},
		},
	}

	resp := th.Handle(req)

	if resp.Header.Status != protocol.StatusOK {
		t.Error("Base handler should be called when tenancy manager is nil")
	}
}

func TestTenancyHandler_extractTenantID_Default(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: []byte("value1"),
				},
			},
		},
	}

	tenantID := th.extractTenantID(req)

	if tenantID != "default" {
		t.Errorf("Expected 'default' tenant ID, got %s", tenantID)
	}
}

func TestTenancyHandler_extractTenantID_FromHeaders(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: []byte("value1"),
					Headers: map[string][]byte{
						"tenant_id": []byte("custom-tenant"),
					},
				},
			},
		},
	}

	tenantID := th.extractTenantID(req)

	if tenantID != "custom-tenant" {
		t.Errorf("Expected 'custom-tenant' tenant ID, got %s", tenantID)
	}
}

func TestTenancyHandler_extractTenantID_FetchRequest(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeFetch,
		},
		Payload: &protocol.FetchRequest{
			Topic: "test-topic",
		},
	}

	tenantID := th.extractTenantID(req)

	// Fetch requests default to "default" tenant
	if tenantID != "default" {
		t.Errorf("Expected 'default' tenant ID for fetch, got %s", tenantID)
	}
}

func TestTenancyHandler_enforceProduceQuota(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant with limited quotas
	quotas := &tenancy.Quotas{
		MaxBytesPerSecond:    100,
		MaxMessagesPerSecond: 10,
		MaxStorageBytes:      1000,
		MaxTopics:            5,
		MaxPartitions:        10,
		MaxConnections:       10,
	}
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: []byte("value1"),
				},
			},
		},
	}

	// Should not error for small request
	err = th.enforceProduceQuota("test-tenant", req)
	if err != nil {
		t.Errorf("Expected no error for small produce, got %v", err)
	}
}

func TestTenancyHandler_enforceProduceQuota_Exceeded(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant with very limited quotas
	quotas := &tenancy.Quotas{
		MaxBytesPerSecond:    10, // Very small limit
		MaxMessagesPerSecond: 1,
		MaxStorageBytes:      1000,
		MaxTopics:            5,
		MaxPartitions:        10,
		MaxConnections:       10,
	}
	_, err := tenancyMgr.CreateTenant("limited-tenant", "Limited Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create a large message that exceeds quota
	largeData := make([]byte, 1000)
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: largeData,
				},
			},
		},
	}

	// Should error due to quota exceeded
	err = th.enforceProduceQuota("limited-tenant", req)
	if err == nil {
		t.Error("Expected quota exceeded error for large produce")
	}

	quotaErr, ok := err.(*tenancy.QuotaError)
	if !ok {
		t.Errorf("Expected QuotaError, got %T", err)
	} else if quotaErr.QuotaType != "bytes_per_second" {
		t.Errorf("Expected bytes_per_second quota error, got %s", quotaErr.QuotaType)
	}
}

func TestTenancyHandler_enforceFetchQuota(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant with quotas
	quotas := tenancy.DefaultQuotas()
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeFetch,
		},
		Payload: &protocol.FetchRequest{
			Topic:    "test-topic",
			MaxBytes: 1024,
		},
	}

	err = th.enforceFetchQuota("test-tenant", req)
	if err != nil {
		t.Errorf("Expected no error for fetch, got %v", err)
	}
}

func TestTenancyHandler_enforceTopicQuota(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant with quotas
	quotas := &tenancy.Quotas{
		MaxBytesPerSecond:    10000,
		MaxMessagesPerSecond: 1000,
		MaxStorageBytes:      1000000,
		MaxTopics:            5,
		MaxPartitions:        10,
		MaxConnections:       10,
	}
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeCreateTopic,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "new-topic",
			NumPartitions: 2,
		},
	}

	err = th.enforceTopicQuota("test-tenant", req)
	if err != nil {
		t.Errorf("Expected no error for topic creation, got %v", err)
	}
}

func TestTenancyHandler_enforceTopicQuota_Exceeded(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant with very limited quotas
	quotas := &tenancy.Quotas{
		MaxBytesPerSecond:    10000,
		MaxMessagesPerSecond: 1000,
		MaxStorageBytes:      1000000,
		MaxTopics:            1, // Only allow 1 topic
		MaxPartitions:        10,
		MaxConnections:       10,
	}
	_, err := tenancyMgr.CreateTenant("limited-tenant", "Limited Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create first topic (should succeed)
	req1 := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeCreateTopic,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "topic-1",
			NumPartitions: 1,
		},
	}
	err = th.enforceTopicQuota("limited-tenant", req1)
	if err != nil {
		t.Fatalf("First topic creation should succeed: %v", err)
	}

	// Try to create second topic (should fail)
	req2 := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeCreateTopic,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "topic-2",
			NumPartitions: 1,
		},
	}
	err = th.enforceTopicQuota("limited-tenant", req2)
	if err == nil {
		t.Error("Expected quota exceeded error for second topic")
	}
}

func TestTenancyHandler_enforceQuotas_OtherRequestTypes(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Test request types that don't have quota enforcement
	testCases := []protocol.RequestType{
		protocol.RequestTypeGetOffset,
		protocol.RequestTypeDeleteTopic,
		protocol.RequestTypeListTopics,
		protocol.RequestTypeHealthCheck,
	}

	for _, reqType := range testCases {
		req := &protocol.Request{
			Header: protocol.RequestHeader{
				Type: reqType,
			},
			Payload: &protocol.GetOffsetRequest{}, // Simple payload
		}

		err := th.enforceQuotas("test-tenant", req)
		if err != nil {
			t.Errorf("Expected no error for request type %v, got %v", reqType, err)
		}
	}
}

func TestTenancyHandler_quotaExceededResponse(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	quotaErr := &tenancy.QuotaError{
		TenantID:  "test-tenant",
		QuotaType: "bytes_per_second",
		Current:   1000,
		Limit:     500,
		Message:   "bytes per second quota exceeded",
	}

	resp := th.quotaExceededResponse(123, "test-tenant", quotaErr)

	if resp.Header.RequestID != 123 {
		t.Errorf("RequestID = %d, want 123", resp.Header.RequestID)
	}

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Header.Status)
	}

	errorResp := resp.Payload.(*protocol.ErrorResponse)
	if errorResp == nil {
		t.Fatal("Expected ErrorResponse payload")
	}

	if errorResp.Message == "" {
		t.Error("Expected error message in response")
	}
}

func TestTenancyHandler_quotaExceededResponse_NonQuotaError(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Pass a non-QuotaError (use fmt.Errorf to create a real error)
	err := fmt.Errorf("generic error")

	resp := th.quotaExceededResponse(456, "test-tenant", err)

	if resp.Header.RequestID != 456 {
		t.Errorf("RequestID = %d, want 456", resp.Header.RequestID)
	}

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrInvalidRequest {
		t.Errorf("ErrorCode = %v, want ErrInvalidRequest", resp.Header.ErrorCode)
	}
}

func TestTenancyHandler_GetStats(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, false)

	// Initially zero
	stats := th.GetStats()
	if stats["requests_handled"] != 0 {
		t.Errorf("requests_handled = %d, want 0", stats["requests_handled"])
	}
	if stats["quota_violations"] != 0 {
		t.Errorf("quota_violations = %d, want 0", stats["quota_violations"])
	}

	// Handle a request (tenancy disabled)
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: []byte("value1"),
				},
			},
		},
	}
	th.Handle(req)

	// Check stats updated
	stats = th.GetStats()
	if stats["requests_handled"] != 1 {
		t.Errorf("requests_handled = %d, want 1", stats["requests_handled"])
	}
}

func TestTenancyHandler_Handle_WithQuotaViolation(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	tenancyMgr := tenancy.NewManager()
	th := NewTenancyHandler(baseHandler, tenancyMgr, true)

	// Create tenant with very limited quotas
	quotas := &tenancy.Quotas{
		MaxBytesPerSecond:    10, // Very small limit
		MaxMessagesPerSecond: 1,
		MaxStorageBytes:      1000,
		MaxTopics:            5,
		MaxPartitions:        10,
		MaxConnections:       10,
	}
	_, err := tenancyMgr.CreateTenant("default", "Default Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create a large message that exceeds quota
	largeData := make([]byte, 1000)
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 789,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{
					Key:   []byte("key1"),
					Value: largeData,
				},
			},
		},
	}

	resp := th.Handle(req)

	if resp.Header.Status != protocol.StatusError {
		t.Error("Expected error status for quota violation")
	}

	stats := th.GetStats()
	if stats["quota_violations"] != 1 {
		t.Errorf("quota_violations = %d, want 1", stats["quota_violations"])
	}
}
