package protocol

import (
	"bytes"
	"testing"
	"time"
)

func TestCodec_ProduceRequest(t *testing.T) {
	codec := NewCodec()

	// Create a produce request
	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeProduce,
			Version:   ProtocolVersion,
			Flags:     FlagRequireAck,
		},
		Payload: &ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []Message{
				{
					Offset:    0,
					Key:       []byte("key1"),
					Value:     []byte("value1"),
					Timestamp: time.Now().UnixNano(),
					Headers: map[string][]byte{
						"header1": []byte("hvalue1"),
					},
				},
				{
					Offset:    1,
					Key:       []byte("key2"),
					Value:     []byte("value2"),
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify header
	if decoded.Header.RequestID != req.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", decoded.Header.RequestID, req.Header.RequestID)
	}
	if decoded.Header.Type != req.Header.Type {
		t.Errorf("Type mismatch: got %v, want %v", decoded.Header.Type, req.Header.Type)
	}

	// Verify payload
	decodedPayload := decoded.Payload.(*ProduceRequest)
	originalPayload := req.Payload.(*ProduceRequest)

	if decodedPayload.Topic != originalPayload.Topic {
		t.Errorf("Topic mismatch: got %s, want %s", decodedPayload.Topic, originalPayload.Topic)
	}
	if decodedPayload.PartitionID != originalPayload.PartitionID {
		t.Errorf("PartitionID mismatch: got %d, want %d", decodedPayload.PartitionID, originalPayload.PartitionID)
	}
	if len(decodedPayload.Messages) != len(originalPayload.Messages) {
		t.Fatalf("Message count mismatch: got %d, want %d", len(decodedPayload.Messages), len(originalPayload.Messages))
	}

	// Verify first message
	msg := decodedPayload.Messages[0]
	origMsg := originalPayload.Messages[0]
	if !bytes.Equal(msg.Key, origMsg.Key) {
		t.Errorf("Key mismatch: got %s, want %s", msg.Key, origMsg.Key)
	}
	if !bytes.Equal(msg.Value, origMsg.Value) {
		t.Errorf("Value mismatch: got %s, want %s", msg.Value, origMsg.Value)
	}
	if msg.Timestamp != origMsg.Timestamp {
		t.Errorf("Timestamp mismatch: got %d, want %d", msg.Timestamp, origMsg.Timestamp)
	}
}

func TestCodec_FetchRequest(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 67890,
			Type:      RequestTypeFetch,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &FetchRequest{
			Topic:       "test-topic",
			PartitionID: 1,
			Offset:      100,
			MaxBytes:    1024 * 1024,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify
	decodedPayload := decoded.Payload.(*FetchRequest)
	originalPayload := req.Payload.(*FetchRequest)

	if decodedPayload.Topic != originalPayload.Topic {
		t.Errorf("Topic mismatch: got %s, want %s", decodedPayload.Topic, originalPayload.Topic)
	}
	if decodedPayload.PartitionID != originalPayload.PartitionID {
		t.Errorf("PartitionID mismatch: got %d, want %d", decodedPayload.PartitionID, originalPayload.PartitionID)
	}
	if decodedPayload.Offset != originalPayload.Offset {
		t.Errorf("Offset mismatch: got %d, want %d", decodedPayload.Offset, originalPayload.Offset)
	}
	if decodedPayload.MaxBytes != originalPayload.MaxBytes {
		t.Errorf("MaxBytes mismatch: got %d, want %d", decodedPayload.MaxBytes, originalPayload.MaxBytes)
	}
}

func TestCodec_GetOffsetRequest(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 111,
			Type:      RequestTypeGetOffset,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &GetOffsetRequest{
			Topic:       "test-topic",
			PartitionID: 2,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify
	decodedPayload := decoded.Payload.(*GetOffsetRequest)
	originalPayload := req.Payload.(*GetOffsetRequest)

	if decodedPayload.Topic != originalPayload.Topic {
		t.Errorf("Topic mismatch: got %s, want %s", decodedPayload.Topic, originalPayload.Topic)
	}
	if decodedPayload.PartitionID != originalPayload.PartitionID {
		t.Errorf("PartitionID mismatch: got %d, want %d", decodedPayload.PartitionID, originalPayload.PartitionID)
	}
}

func TestCodec_CreateTopicRequest(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 222,
			Type:      RequestTypeCreateTopic,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &CreateTopicRequest{
			Topic:             "new-topic",
			NumPartitions:     4,
			ReplicationFactor: 3,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify
	decodedPayload := decoded.Payload.(*CreateTopicRequest)
	originalPayload := req.Payload.(*CreateTopicRequest)

	if decodedPayload.Topic != originalPayload.Topic {
		t.Errorf("Topic mismatch: got %s, want %s", decodedPayload.Topic, originalPayload.Topic)
	}
	if decodedPayload.NumPartitions != originalPayload.NumPartitions {
		t.Errorf("NumPartitions mismatch: got %d, want %d", decodedPayload.NumPartitions, originalPayload.NumPartitions)
	}
	if decodedPayload.ReplicationFactor != originalPayload.ReplicationFactor {
		t.Errorf("ReplicationFactor mismatch: got %d, want %d", decodedPayload.ReplicationFactor, originalPayload.ReplicationFactor)
	}
}

func TestCodec_DeleteTopicRequest(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 333,
			Type:      RequestTypeDeleteTopic,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &DeleteTopicRequest{
			Topic: "old-topic",
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify
	decodedPayload := decoded.Payload.(*DeleteTopicRequest)
	originalPayload := req.Payload.(*DeleteTopicRequest)

	if decodedPayload.Topic != originalPayload.Topic {
		t.Errorf("Topic mismatch: got %s, want %s", decodedPayload.Topic, originalPayload.Topic)
	}
}

func TestCodec_ListTopicsRequest(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 444,
			Type:      RequestTypeListTopics,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &ListTopicsRequest{},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify header
	if decoded.Header.RequestID != req.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", decoded.Header.RequestID, req.Header.RequestID)
	}
	if decoded.Header.Type != req.Header.Type {
		t.Errorf("Type mismatch: got %v, want %v", decoded.Header.Type, req.Header.Type)
	}
}

func TestCodec_HealthCheckRequest(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 555,
			Type:      RequestTypeHealthCheck,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &HealthCheckRequest{},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify header
	if decoded.Header.RequestID != req.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", decoded.Header.RequestID, req.Header.RequestID)
	}
	if decoded.Header.Type != req.Header.Type {
		t.Errorf("Type mismatch: got %v, want %v", decoded.Header.Type, req.Header.Type)
	}
}

func TestCodec_ErrorResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 999,
			Status:    StatusError,
			ErrorCode: ErrPartitionNotFound,
		},
		Payload: &ErrorResponse{
			ErrorCode: ErrPartitionNotFound,
			Message:   "Partition not found: test-topic-0",
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("Failed to encode response: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify header
	if decoded.Header.RequestID != resp.Header.RequestID {
		t.Errorf("RequestID mismatch: got %d, want %d", decoded.Header.RequestID, resp.Header.RequestID)
	}
	if decoded.Header.Status != resp.Header.Status {
		t.Errorf("Status mismatch: got %v, want %v", decoded.Header.Status, resp.Header.Status)
	}
	if decoded.Header.ErrorCode != resp.Header.ErrorCode {
		t.Errorf("ErrorCode mismatch: got %v, want %v", decoded.Header.ErrorCode, resp.Header.ErrorCode)
	}

	// Verify payload
	decodedPayload := decoded.Payload.(*ErrorResponse)
	originalPayload := resp.Payload.(*ErrorResponse)

	if decodedPayload.ErrorCode != originalPayload.ErrorCode {
		t.Errorf("ErrorCode mismatch: got %v, want %v", decodedPayload.ErrorCode, originalPayload.ErrorCode)
	}
	if decodedPayload.Message != originalPayload.Message {
		t.Errorf("Message mismatch: got %s, want %s", decodedPayload.Message, originalPayload.Message)
	}
}

func TestCodec_ChecksumMismatch(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeFetch,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &FetchRequest{
			Topic:       "test",
			PartitionID: 0,
			Offset:      0,
			MaxBytes:    1024,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Corrupt the message (change a byte in the payload)
	data := buf.Bytes()
	data[30] ^= 0xFF // Flip bits in the middle of the message

	// Try to decode
	corruptedBuf := bytes.NewBuffer(data)
	_, err = codec.DecodeRequest(corruptedBuf)
	if err != ErrChecksumMismatch {
		t.Errorf("Expected ErrChecksumMismatch, got %v", err)
	}
}

func TestCodec_MessageTooLarge(t *testing.T) {
	codec := NewCodec()

	// Create a message that's too large
	largeValue := make([]byte, MaxMessageSize+1)
	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeProduce,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &ProduceRequest{
			Topic:       "test",
			PartitionID: 0,
			Messages: []Message{
				{
					Key:   []byte("key"),
					Value: largeValue,
				},
			},
		},
	}

	// Try to encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err == nil {
		t.Fatal("Expected error for message too large, got nil")
	}
}

func TestCodec_EmptyMessage(t *testing.T) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeProduce,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &ProduceRequest{
			Topic:       "test",
			PartitionID: 0,
			Messages: []Message{
				{
					Key:       nil,
					Value:     []byte("value"),
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify
	decodedPayload := decoded.Payload.(*ProduceRequest)
	if len(decodedPayload.Messages[0].Key) != 0 {
		t.Errorf("Expected empty key, got %v", decodedPayload.Messages[0].Key)
	}
}

func TestCodec_MessageWithHeaders(t *testing.T) {
	codec := NewCodec()

	headers := map[string][]byte{
		"content-type": []byte("application/json"),
		"user-id":      []byte("12345"),
		"trace-id":     []byte("abc-def-ghi"),
	}

	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeProduce,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &ProduceRequest{
			Topic:       "test",
			PartitionID: 0,
			Messages: []Message{
				{
					Key:       []byte("key"),
					Value:     []byte("value"),
					Headers:   headers,
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeRequest(buf, req)
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Verify headers
	decodedPayload := decoded.Payload.(*ProduceRequest)
	decodedHeaders := decodedPayload.Messages[0].Headers

	if len(decodedHeaders) != len(headers) {
		t.Fatalf("Header count mismatch: got %d, want %d", len(decodedHeaders), len(headers))
	}

	for k, v := range headers {
		decodedV, ok := decodedHeaders[k]
		if !ok {
			t.Errorf("Missing header: %s", k)
			continue
		}
		if !bytes.Equal(decodedV, v) {
			t.Errorf("Header value mismatch for %s: got %s, want %s", k, decodedV, v)
		}
	}
}

// Benchmarks

func BenchmarkCodec_EncodeProduceRequest(b *testing.B) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeProduce,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []Message{
				{
					Key:       []byte("key"),
					Value:     []byte("value"),
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	buf := &bytes.Buffer{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		codec.EncodeRequest(buf, req)
	}
}

func BenchmarkCodec_DecodeProduceRequest(b *testing.B) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 12345,
			Type:      RequestTypeProduce,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &ProduceRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Messages: []Message{
				{
					Key:       []byte("key"),
					Value:     []byte("value"),
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	buf := &bytes.Buffer{}
	codec.EncodeRequest(buf, req)
	data := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		codec.DecodeRequest(bytes.NewBuffer(data))
	}
}

func BenchmarkCodec_EncodeFetchRequest(b *testing.B) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 67890,
			Type:      RequestTypeFetch,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &FetchRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Offset:      100,
			MaxBytes:    1024 * 1024,
		},
	}

	buf := &bytes.Buffer{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		codec.EncodeRequest(buf, req)
	}
}

func BenchmarkCodec_DecodeFetchRequest(b *testing.B) {
	codec := NewCodec()

	req := &Request{
		Header: RequestHeader{
			RequestID: 67890,
			Type:      RequestTypeFetch,
			Version:   ProtocolVersion,
			Flags:     FlagNone,
		},
		Payload: &FetchRequest{
			Topic:       "test-topic",
			PartitionID: 0,
			Offset:      100,
			MaxBytes:    1024 * 1024,
		},
	}

	buf := &bytes.Buffer{}
	codec.EncodeRequest(buf, req)
	data := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		codec.DecodeRequest(bytes.NewBuffer(data))
	}
}


// Additional response encoding/decoding tests for coverage

func TestCodec_ProduceResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 123,
			Status:    StatusOK,
		},
		Payload: &ProduceResponse{
			BaseOffset:    100,
			NumMessages:   5,
			HighWaterMark: 105,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeProduce)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	produceResp := decoded.Payload.(*ProduceResponse)
	if produceResp.BaseOffset != 100 {
		t.Errorf("BaseOffset = %d, want 100", produceResp.BaseOffset)
	}
	if produceResp.NumMessages != 5 {
		t.Errorf("NumMessages = %d, want 5", produceResp.NumMessages)
	}
	if produceResp.HighWaterMark != 105 {
		t.Errorf("HighWaterMark = %d, want 105", produceResp.HighWaterMark)
	}
}

func TestCodec_FetchResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 124,
			Status:    StatusOK,
		},
		Payload: &FetchResponse{
			Messages: []Message{
				{
					Offset:    100,
					Key:       []byte("key1"),
					Value:     []byte("value1"),
					Timestamp: time.Now().UnixNano(),
				},
				{
					Offset:    101,
					Key:       []byte("key2"),
					Value:     []byte("value2"),
					Timestamp: time.Now().UnixNano(),
				},
			},
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeFetch)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	fetchResp := decoded.Payload.(*FetchResponse)
	if len(fetchResp.Messages) != 2 {
		t.Errorf("Messages count = %d, want 2", len(fetchResp.Messages))
	}
}

func TestCodec_GetOffsetResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 125,
			Status:    StatusOK,
		},
		Payload: &GetOffsetResponse{
			Topic:         "test-topic",
			PartitionID:   0,
			StartOffset:   999,
			EndOffset:     1100,
			HighWaterMark: 1000,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeGetOffset)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	offsetResp := decoded.Payload.(*GetOffsetResponse)
	if offsetResp.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", offsetResp.Topic)
	}
	if offsetResp.StartOffset != 999 {
		t.Errorf("StartOffset = %d, want 999", offsetResp.StartOffset)
	}
	if offsetResp.EndOffset != 1100 {
		t.Errorf("EndOffset = %d, want 1100", offsetResp.EndOffset)
	}
	if offsetResp.HighWaterMark != 1000 {
		t.Errorf("HighWaterMark = %d, want 1000", offsetResp.HighWaterMark)
	}
}

func TestCodec_CreateTopicResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 126,
			Status:    StatusOK,
		},
		Payload: &CreateTopicResponse{
			Created: true,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeCreateTopic)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	createResp := decoded.Payload.(*CreateTopicResponse)
	if !createResp.Created {
		t.Errorf("Created = false, want true")
	}
}

func TestCodec_DeleteTopicResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 127,
			Status:    StatusOK,
		},
		Payload: &DeleteTopicResponse{
			Deleted: true,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeDeleteTopic)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	deleteResp := decoded.Payload.(*DeleteTopicResponse)
	if !deleteResp.Deleted {
		t.Errorf("Deleted = false, want true")
	}
}

func TestCodec_ListTopicsResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 128,
			Status:    StatusOK,
		},
		Payload: &ListTopicsResponse{
			Topics: []TopicInfo{
				{Name: "topic1", NumPartitions: 3},
				{Name: "topic2", NumPartitions: 5},
				{Name: "topic3", NumPartitions: 1},
			},
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeListTopics)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	listResp := decoded.Payload.(*ListTopicsResponse)
	if len(listResp.Topics) != 3 {
		t.Errorf("Topics count = %d, want 3", len(listResp.Topics))
	}
	if listResp.Topics[0].Name != "topic1" {
		t.Errorf("Topics[0].Name = %s, want topic1", listResp.Topics[0].Name)
	}
}

func TestCodec_HealthCheckResponse(t *testing.T) {
	codec := NewCodec()

	resp := &Response{
		Header: ResponseHeader{
			RequestID: 129,
			Status:    StatusOK,
		},
		Payload: &HealthCheckResponse{
			Status: "healthy",
			Uptime: 3600,
		},
	}

	// Encode
	buf := &bytes.Buffer{}
	err := codec.EncodeResponse(buf, resp)
	if err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	// Decode
	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}

	// Decode payload
	err = codec.DecodeResponsePayload(decoded, RequestTypeHealthCheck)
	if err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	healthResp := decoded.Payload.(*HealthCheckResponse)
	if healthResp.Status != "healthy" {
		t.Errorf("Status = %s, want healthy", healthResp.Status)
	}
	if healthResp.Uptime != 3600 {
		t.Errorf("Uptime = %d, want 3600", healthResp.Uptime)
	}
}
