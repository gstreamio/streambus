package server

import (
	"bytes"
	"testing"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// TestHandler_ProduceFetch_RoundTripsHeaders guards a regression in which the
// handler built its storage.Message on produce, and its protocol.Message on
// fetch, without copying Headers. The record format has carried headers since
// v2 and the storage layer persisted them correctly, so nothing failed
// loudly - every header a producer set was simply gone by the time a consumer
// read the record back.
//
// This matters beyond user-set headers: the tenancy handler reads tenant_id
// out of them.
func TestHandler_ProduceFetch_RoundTripsHeaders(t *testing.T) {
	handler := NewHandlerWithDataDir(t.TempDir())
	defer handler.Close()

	const topic = "header-round-trip"

	createResp := handler.handleCreateTopic(&protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{Topic: topic, NumPartitions: 1},
	})
	if createResp.Header.Status != protocol.StatusOK {
		t.Fatalf("CreateTopic failed: %v", createResp.Payload)
	}

	want := map[string][]byte{
		"tenant_id":    []byte("acme"),
		"content-type": []byte("application/json"),
	}

	produceResp := handler.handleProduce(&protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       topic,
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k"), Value: []byte("v"), Headers: want, Timestamp: 1},
				{Key: []byte("no-headers"), Value: []byte("v2"), Timestamp: 2},
			},
		},
	})
	if produceResp.Header.Status != protocol.StatusOK {
		t.Fatalf("Produce failed: %v", produceResp.Payload)
	}

	fetchResp := handler.handleFetch(&protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 3,
			Type:      protocol.RequestTypeFetch,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.FetchRequest{
			Topic:       topic,
			PartitionID: 0,
			Offset:      0,
			MaxBytes:    1024 * 1024,
		},
	})
	if fetchResp.Header.Status != protocol.StatusOK {
		t.Fatalf("Fetch failed: %v", fetchResp.Payload)
	}

	fetched, ok := fetchResp.Payload.(*protocol.FetchResponse)
	if !ok {
		t.Fatalf("expected *protocol.FetchResponse, got %T", fetchResp.Payload)
	}
	if len(fetched.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(fetched.Messages))
	}

	got := fetched.Messages[0].Headers
	if len(got) != len(want) {
		t.Fatalf("expected %d headers, got %d (%v)", len(want), len(got), got)
	}
	for k, wantVal := range want {
		gotVal, present := got[k]
		if !present {
			t.Errorf("header %q missing from the fetched message", k)
			continue
		}
		if !bytes.Equal(gotVal, wantVal) {
			t.Errorf("header %q = %q, want %q", k, gotVal, wantVal)
		}
	}

	// A message written without headers must not acquire any on the way back.
	if len(fetched.Messages[1].Headers) != 0 {
		t.Errorf("expected no headers on the second message, got %v", fetched.Messages[1].Headers)
	}
}
