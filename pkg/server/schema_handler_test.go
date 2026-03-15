package server

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/schema"
)

// helper to create a schema registry with a validator
func newTestSchemaRegistry() *schema.SchemaRegistry {
	validator := schema.NewDefaultValidator()
	return schema.NewSchemaRegistry(validator, nil)
}

func TestSchemaHandler_ProduceSucceeds_NoSchemaRegistered(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Create topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "no-schema-topic",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Produce should succeed with any data when no schema is registered
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "no-schema-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("key1"), Value: []byte("any arbitrary data")},
			},
		},
	}

	resp := handler.Handle(produceReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("expected OK status, got %v: %v", resp.Header.Status, resp.Payload)
	}

	stats := handler.GetStats()
	if stats["validation_failures"] != 0 {
		t.Errorf("expected 0 validation failures, got %d", stats["validation_failures"])
	}
}

func TestSchemaHandler_ProduceSucceeds_MessageMatchesJSONSchema(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Register a JSON schema for topic-value
	jsonSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name"]
	}`
	regResp, err := registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "json-topic-value",
		Format:     schema.FormatJSON,
		Definition: jsonSchema,
	})
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}
	if regResp.ErrorCode != schema.ErrorNone {
		t.Fatalf("schema registration error: %v", regResp.ErrorCode)
	}

	// Create topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "json-topic",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Produce a valid message
	validJSON := `{"name": "Alice", "age": 30}`
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "json-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k1"), Value: []byte(validJSON), Timestamp: time.Now().UnixNano()},
			},
		},
	}

	resp := handler.Handle(produceReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("expected OK status for valid message, got %v: %v", resp.Header.Status, resp.Payload)
	}
}

func TestSchemaHandler_ProduceFails_MessageViolatesJSONSchema(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Register a JSON schema requiring "name" field
	jsonSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name"]
	}`
	_, err := registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "strict-topic-value",
		Format:     schema.FormatJSON,
		Definition: jsonSchema,
	})
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	// Create topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "strict-topic",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Produce message missing required field "name"
	invalidJSON := `{"age": 30}`
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "strict-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k1"), Value: []byte(invalidJSON)},
			},
		},
	}

	resp := handler.Handle(produceReq)
	if resp.Header.Status != protocol.StatusError {
		t.Fatalf("expected error status for invalid message, got %v", resp.Header.Status)
	}
	if resp.Header.ErrorCode != protocol.ErrSchemaValidationFailed {
		t.Errorf("expected ErrSchemaValidationFailed, got %v", resp.Header.ErrorCode)
	}

	// Verify error message mentions the missing field
	errResp := resp.Payload.(*protocol.ErrorResponse)
	if errResp.Message == "" {
		t.Error("expected error message to be non-empty")
	}

	stats := handler.GetStats()
	if stats["validation_failures"] != 1 {
		t.Errorf("expected 1 validation failure, got %d", stats["validation_failures"])
	}
}

func TestSchemaHandler_ProduceFails_NotJSON(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Register a JSON schema
	jsonSchema := `{"type": "object", "properties": {"name": {"type": "string"}}}`
	_, _ = registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "json-required-topic-value",
		Format:     schema.FormatJSON,
		Definition: jsonSchema,
	})

	// Produce non-JSON data
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "json-required-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k1"), Value: []byte("this is not json")},
			},
		},
	}

	resp := handler.Handle(produceReq)
	if resp.Header.Status != protocol.StatusError {
		t.Fatalf("expected error for non-JSON data, got %v", resp.Header.Status)
	}
	if resp.Header.ErrorCode != protocol.ErrSchemaValidationFailed {
		t.Errorf("expected ErrSchemaValidationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestSchemaHandler_AvroSchemaValidation(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Register an Avro record schema
	avroSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "username", "type": "string"},
			{"name": "email", "type": "string"}
		]
	}`
	_, err := registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "avro-topic-value",
		Format:     schema.FormatAvro,
		Definition: avroSchema,
	})
	if err != nil {
		t.Fatalf("failed to register Avro schema: %v", err)
	}

	// Create topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "avro-topic",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	t.Run("valid avro message", func(t *testing.T) {
		validMsg := `{"username": "alice", "email": "alice@example.com"}`
		produceReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: 10,
				Type:      protocol.RequestTypeProduce,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.ProduceRequest{
				Topic:       "avro-topic",
				PartitionID: 0,
				Messages: []protocol.Message{
					{Key: []byte("k1"), Value: []byte(validMsg)},
				},
			},
		}

		resp := handler.Handle(produceReq)
		if resp.Header.Status != protocol.StatusOK {
			t.Fatalf("expected OK for valid Avro message, got %v: %v", resp.Header.Status, resp.Payload)
		}
	})

	t.Run("invalid avro message missing field", func(t *testing.T) {
		invalidMsg := `{"username": "bob"}`
		produceReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: 11,
				Type:      protocol.RequestTypeProduce,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.ProduceRequest{
				Topic:       "avro-topic",
				PartitionID: 0,
				Messages: []protocol.Message{
					{Key: []byte("k2"), Value: []byte(invalidMsg)},
				},
			},
		}

		resp := handler.Handle(produceReq)
		if resp.Header.Status != protocol.StatusError {
			t.Fatalf("expected error for invalid Avro message, got %v", resp.Header.Status)
		}
		if resp.Header.ErrorCode != protocol.ErrSchemaValidationFailed {
			t.Errorf("expected ErrSchemaValidationFailed, got %v", resp.Header.ErrorCode)
		}
	})
}

func TestSchemaHandler_KeySchemaValidation(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Register key schema
	keySchema := `{"type": "object", "properties": {"id": {"type": "number"}}, "required": ["id"]}`
	_, _ = registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "key-topic-key",
		Format:     schema.FormatJSON,
		Definition: keySchema,
	})

	// Register value schema
	valueSchema := `{"type": "object", "properties": {"data": {"type": "string"}}}`
	_, _ = registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "key-topic-value",
		Format:     schema.FormatJSON,
		Definition: valueSchema,
	})

	t.Run("valid key and value", func(t *testing.T) {
		produceReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: 1,
				Type:      protocol.RequestTypeProduce,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.ProduceRequest{
				Topic:       "key-topic",
				PartitionID: 0,
				Messages: []protocol.Message{
					{Key: []byte(`{"id": 1}`), Value: []byte(`{"data": "hello"}`)},
				},
			},
		}

		resp := handler.Handle(produceReq)
		if resp.Header.Status != protocol.StatusOK {
			t.Fatalf("expected OK for valid key+value, got %v: %v", resp.Header.Status, resp.Payload)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		produceReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: 2,
				Type:      protocol.RequestTypeProduce,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.ProduceRequest{
				Topic:       "key-topic",
				PartitionID: 0,
				Messages: []protocol.Message{
					{Key: []byte(`{"wrong": "field"}`), Value: []byte(`{"data": "hello"}`)},
				},
			},
		}

		resp := handler.Handle(produceReq)
		if resp.Header.Status != protocol.StatusError {
			t.Fatalf("expected error for invalid key, got %v", resp.Header.Status)
		}
		if resp.Header.ErrorCode != protocol.ErrSchemaValidationFailed {
			t.Errorf("expected ErrSchemaValidationFailed, got %v", resp.Header.ErrorCode)
		}
	})

	t.Run("empty key skips key validation", func(t *testing.T) {
		produceReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: 3,
				Type:      protocol.RequestTypeProduce,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.ProduceRequest{
				Topic:       "key-topic",
				PartitionID: 0,
				Messages: []protocol.Message{
					{Key: nil, Value: []byte(`{"data": "hello"}`)},
				},
			},
		}

		resp := handler.Handle(produceReq)
		if resp.Header.Status != protocol.StatusOK {
			t.Fatalf("expected OK for nil key, got %v: %v", resp.Header.Status, resp.Payload)
		}
	})
}

func TestSchemaHandler_Disabled(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()

	// Register a schema but disable the handler
	jsonSchema := `{"type": "object", "properties": {"name": {"type": "string"}}, "required": ["name"]}`
	_, _ = registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "disabled-topic-value",
		Format:     schema.FormatJSON,
		Definition: jsonSchema,
	})

	handler := NewSchemaHandler(baseHandler, registry, false) // disabled

	// Produce invalid data -- should pass because handler is disabled
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "disabled-topic",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k"), Value: []byte("not json at all")},
			},
		},
	}

	resp := handler.Handle(produceReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("expected OK when schema handler is disabled, got %v", resp.Header.Status)
	}
}

func TestSchemaHandler_NonProduceRequestsPassThrough(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Non-produce requests should pass through without schema validation
	tests := []struct {
		name    string
		reqType protocol.RequestType
		payload interface{}
	}{
		{
			name:    "HealthCheck",
			reqType: protocol.RequestTypeHealthCheck,
			payload: &protocol.HealthCheckRequest{},
		},
		{
			name:    "ListTopics",
			reqType: protocol.RequestTypeListTopics,
			payload: &protocol.ListTopicsRequest{},
		},
		{
			name:    "CreateTopic",
			reqType: protocol.RequestTypeCreateTopic,
			payload: &protocol.CreateTopicRequest{Topic: "pass-through", NumPartitions: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &protocol.Request{
				Header: protocol.RequestHeader{
					RequestID: 1,
					Type:      tc.reqType,
					Version:   protocol.ProtocolVersion,
				},
				Payload: tc.payload,
			}

			resp := handler.Handle(req)
			if resp.Header.Status != protocol.StatusOK {
				t.Errorf("expected OK for %s, got %v", tc.name, resp.Header.Status)
			}
		})
	}
}

func TestSchemaHandler_MultipleSchemasWork(t *testing.T) {
	tempDir := t.TempDir()
	baseHandler := NewHandlerWithDataDir(tempDir)
	defer baseHandler.Close()

	registry := newTestSchemaRegistry()
	handler := NewSchemaHandler(baseHandler, registry, true)

	// Register JSON schema for topic A
	jsonSchema := `{
		"type": "object",
		"properties": {"name": {"type": "string"}},
		"required": ["name"]
	}`
	_, _ = registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "topicA-value",
		Format:     schema.FormatJSON,
		Definition: jsonSchema,
	})

	// Register Avro schema for topic B
	avroSchema := `{
		"type": "record",
		"name": "Event",
		"fields": [
			{"name": "event_type", "type": "string"},
			{"name": "timestamp", "type": "long"}
		]
	}`
	_, _ = registry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    "topicB-value",
		Format:     schema.FormatAvro,
		Definition: avroSchema,
	})

	// Create topics
	for _, topic := range []string{"topicA", "topicB"} {
		handler.Handle(&protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: 1,
				Type:      protocol.RequestTypeCreateTopic,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.CreateTopicRequest{Topic: topic, NumPartitions: 1},
		})
	}

	t.Run("JSON schema topic valid", func(t *testing.T) {
		resp := handler.Handle(&protocol.Request{
			Header: protocol.RequestHeader{RequestID: 10, Type: protocol.RequestTypeProduce, Version: protocol.ProtocolVersion},
			Payload: &protocol.ProduceRequest{
				Topic: "topicA", PartitionID: 0,
				Messages: []protocol.Message{{Value: []byte(`{"name": "test"}`)}},
			},
		})
		if resp.Header.Status != protocol.StatusOK {
			t.Fatalf("expected OK, got %v: %v", resp.Header.Status, resp.Payload)
		}
	})

	t.Run("JSON schema topic invalid", func(t *testing.T) {
		resp := handler.Handle(&protocol.Request{
			Header: protocol.RequestHeader{RequestID: 11, Type: protocol.RequestTypeProduce, Version: protocol.ProtocolVersion},
			Payload: &protocol.ProduceRequest{
				Topic: "topicA", PartitionID: 0,
				Messages: []protocol.Message{{Value: []byte(`{"age": 25}`)}},
			},
		})
		if resp.Header.Status != protocol.StatusError {
			t.Fatalf("expected error, got %v", resp.Header.Status)
		}
	})

	t.Run("Avro schema topic valid", func(t *testing.T) {
		resp := handler.Handle(&protocol.Request{
			Header: protocol.RequestHeader{RequestID: 12, Type: protocol.RequestTypeProduce, Version: protocol.ProtocolVersion},
			Payload: &protocol.ProduceRequest{
				Topic: "topicB", PartitionID: 0,
				Messages: []protocol.Message{{Value: []byte(`{"event_type": "click", "timestamp": 1234567890}`)}},
			},
		})
		if resp.Header.Status != protocol.StatusOK {
			t.Fatalf("expected OK, got %v: %v", resp.Header.Status, resp.Payload)
		}
	})

	t.Run("Avro schema topic invalid", func(t *testing.T) {
		resp := handler.Handle(&protocol.Request{
			Header: protocol.RequestHeader{RequestID: 13, Type: protocol.RequestTypeProduce, Version: protocol.ProtocolVersion},
			Payload: &protocol.ProduceRequest{
				Topic: "topicB", PartitionID: 0,
				Messages: []protocol.Message{{Value: []byte(`{"event_type": "click"}`)}},
			},
		})
		if resp.Header.Status != protocol.StatusError {
			t.Fatalf("expected error for missing Avro field, got %v", resp.Header.Status)
		}
	})
}
