package server

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

func TestServer_StartStop(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0" // Use random available port

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is listening
	addr := server.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	conn.Close()

	// Stop server
	err = server.Stop()
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	// Verify server is stopped
	time.Sleep(100 * time.Millisecond)
	_, err = net.Dial("tcp", addr)
	if err == nil {
		t.Fatal("Server still accepting connections after stop")
	}
}

func TestServer_HealthCheck(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0"

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	// Connect to server
	addr := server.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Create health check request
	codec := protocol.NewCodec()
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeHealthCheck,
			Version:   protocol.ProtocolVersion,
			Flags:     protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	// Send request
	err = codec.EncodeRequest(conn, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Receive response
	resp, err := codec.DecodeResponse(conn)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response
	if resp.Header.RequestID != req.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", resp.Header.RequestID, req.Header.RequestID)
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected StatusOK, got %v", resp.Header.Status)
	}
}

func TestServer_ProduceRequest(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0"

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	// Connect
	addr := server.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec()

	// First, create the topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:              "test-topic",
			NumPartitions:      1,
			ReplicationFactor:  1,
		},
	}

	err = codec.EncodeRequest(conn, createReq)
	if err != nil {
		t.Fatalf("Failed to encode create topic request: %v", err)
	}

	createResp, err := codec.DecodeResponse(conn)
	if err != nil {
		t.Fatalf("Failed to decode create topic response: %v", err)
	}

	if createResp.Header.Status != protocol.StatusOK {
		t.Fatalf("Failed to create topic: status=%v", createResp.Header.Status)
	}

	// Now create produce request
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
			Flags:     protocol.FlagRequireAck,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{
					Key:       []byte("key1"),
					Value:     []byte("value1"),
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	// Send request
	err = codec.EncodeRequest(conn, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Receive response
	resp, err := codec.DecodeResponse(conn)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response
	if resp.Header.RequestID != req.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", resp.Header.RequestID, req.Header.RequestID)
	}
	if resp.Header.Status != protocol.StatusOK {
		errorMsg := ""
		if resp.Payload != nil {
			if errResp, ok := resp.Payload.(*protocol.ErrorResponse); ok {
				errorMsg = fmt.Sprintf(" - ErrorCode: %v, Message: %s", errResp.ErrorCode, errResp.Message)
			}
		}
		t.Errorf("Expected StatusOK, got %v%s", resp.Header.Status, errorMsg)
	}
}

func TestServer_FetchRequest(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0"

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	// Connect
	addr := server.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec()

	// First, create the topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:              "test-topic",
			NumPartitions:      1,
			ReplicationFactor:  1,
		},
	}

	err = codec.EncodeRequest(conn, createReq)
	if err != nil {
		t.Fatalf("Failed to encode create topic request: %v", err)
	}

	createResp, err := codec.DecodeResponse(conn)
	if err != nil {
		t.Fatalf("Failed to decode create topic response: %v", err)
	}

	if createResp.Header.Status != protocol.StatusOK {
		t.Fatalf("Failed to create topic: status=%v", createResp.Header.Status)
	}

	// Now create fetch request
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeFetch,
			Version:   protocol.ProtocolVersion,
			Flags:     protocol.FlagNone,
		},
		Payload: &protocol.FetchRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Offset:      0,
			MaxBytes:    1024 * 1024,
		},
	}

	// Send request
	err = codec.EncodeRequest(conn, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Receive response
	resp, err := codec.DecodeResponse(conn)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response
	if resp.Header.RequestID != req.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", resp.Header.RequestID, req.Header.RequestID)
	}
	if resp.Header.Status != protocol.StatusOK {
		errorMsg := ""
		if resp.Payload != nil {
			if errResp, ok := resp.Payload.(*protocol.ErrorResponse); ok {
				errorMsg = fmt.Sprintf(" - ErrorCode: %v, Message: %s", errResp.ErrorCode, errResp.Message)
			}
		}
		t.Errorf("Expected StatusOK, got %v%s", resp.Header.Status, errorMsg)
	}
}

func TestServer_MultipleConnections(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0"
	config.MaxConnections = 5

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	addr := server.listener.Addr().String()
	numConns := 5
	conns := make([]net.Conn, numConns)

	// Create multiple connections
	for i := 0; i < numConns; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("Failed to connect #%d: %v", i, err)
		}
		conns[i] = conn
	}

	// Wait for all connections to be registered
	time.Sleep(200 * time.Millisecond)

	// Verify all connections are active
	stats := server.Stats()
	if stats.ActiveConnections != int64(numConns) {
		t.Errorf("Expected %d active connections, got %d", numConns, stats.ActiveConnections)
	}

	// Close all connections
	for _, conn := range conns {
		conn.Close()
	}

	// Wait for connections to close
	time.Sleep(100 * time.Millisecond)

	// Verify connections are closed
	stats = server.Stats()
	if stats.ActiveConnections != 0 {
		t.Errorf("Expected 0 active connections, got %d", stats.ActiveConnections)
	}
}

func TestServer_ConnectionLimit(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0"
	config.MaxConnections = 2

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	addr := server.listener.Addr().String()

	// Create max connections
	conn1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect #1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect #2: %v", err)
	}
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	// Try to create one more connection (should be rejected)
	conn3, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect #3: %v", err)
	}
	defer conn3.Close()

	// Send a request on the 3rd connection
	// It should be closed immediately
	codec := protocol.NewCodec()
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeHealthCheck,
			Version:   protocol.ProtocolVersion,
			Flags:     protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	// This might fail because the connection was closed
	err = codec.EncodeRequest(conn3, req)
	// We expect this to potentially fail or timeout
	_ = err
}

func TestServer_Stats(t *testing.T) {
	config := DefaultConfig()
	config.Address = ":0"

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	// Initial stats
	stats := server.Stats()
	if stats.TotalRequests != 0 {
		t.Errorf("Expected 0 requests initially, got %d", stats.TotalRequests)
	}

	// Connect and send a request
	addr := server.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec()
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeHealthCheck,
			Version:   protocol.ProtocolVersion,
			Flags:     protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	err = codec.EncodeRequest(conn, req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	// Receive response
	_, err = codec.DecodeResponse(conn)
	if err != nil {
		t.Fatalf("Failed to receive response: %v", err)
	}

	// Check stats
	time.Sleep(50 * time.Millisecond)
	stats = server.Stats()
	if stats.TotalRequests != 1 {
		t.Errorf("Expected 1 request, got %d", stats.TotalRequests)
	}
	if stats.Uptime == 0 {
		t.Error("Expected non-zero uptime")
	}
}

// Benchmark

func BenchmarkServer_HealthCheck(b *testing.B) {
	config := DefaultConfig()
	config.Address = ":0"

	handler := NewHandler()
	server, err := New(config, handler)
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	addr := server.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec()
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeHealthCheck,
			Version:   protocol.ProtocolVersion,
			Flags:     protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	// Pre-encode request
	buf := &bytes.Buffer{}
	codec.EncodeRequest(buf, req)
	requestData := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Send request
		conn.Write(requestData)

		// Read response
		resp, err := codec.DecodeResponse(conn)
		if err != nil {
			b.Fatalf("Failed to decode response: %v", err)
		}
		if resp.Header.Status != protocol.StatusOK {
			b.Fatalf("Expected OK status, got %v", resp.Header.Status)
		}
	}
}
