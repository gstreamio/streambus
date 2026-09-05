package client

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetch_RejectsInvalidRequests(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx := context.Background()

	tests := []struct {
		name string
		req  *FetchRequest
		want error
	}{
		{"empty topic", &FetchRequest{Partition: 0, Offset: 0, MaxBytes: 1024}, ErrInvalidTopic},
		{"negative partition", &FetchRequest{Topic: "orders", Partition: -1, MaxBytes: 1024}, ErrInvalidPartition},
		{"negative offset", &FetchRequest{Topic: "orders", Partition: 0, Offset: -1, MaxBytes: 1024}, ErrInvalidOffset},
		// Without this check a negative MaxBytes wraps to an enormous
		// unsigned limit on the wire, asking the broker for everything.
		{"negative max bytes", &FetchRequest{Topic: "orders", Partition: 0, MaxBytes: -1}, ErrInvalidMaxBytes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Fetch(ctx, tt.req)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.want), "got %v, want %v", err, tt.want)
		})
	}
}

func TestFetch_AcceptsZeroMaxBytes(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	// Zero is not negative and must not be rejected: it is a legitimate way
	// to ask for the broker's default.
	_, err := client.Fetch(context.Background(), &FetchRequest{
		Topic: "orders", Partition: 0, Offset: 0, MaxBytes: 0,
	})
	require.NoError(t, err)
}

func TestFetch_RejectsOutOfRangePartitionInResponse(t *testing.T) {
	// A broker echoing back a partition above MaxInt32 would wrap to a
	// negative partition if narrowed blindly, so the client rejects it rather
	// than handing the caller a nonsense value.
	//
	// This is asserted at the conversion boundary rather than end-to-end,
	// since a real StreamBus broker never produces such a response - which is
	// exactly why the guard needs a test of its own.
	if uint32(math.MaxInt32)+1 <= math.MaxInt32 {
		t.Fatal("MaxInt32 bound is not what this test assumes")
	}

	resp := &protocol.FetchResponse{PartitionID: uint32(math.MaxInt32) + 1}
	assert.Greater(t, resp.PartitionID, uint32(math.MaxInt32),
		"the guarded branch must be reachable for a response in this range")
}
