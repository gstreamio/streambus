package protocol

import (
	"reflect"
	"testing"
)

func TestFindCoordinatorRequest_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload *FindCoordinatorRequest
	}{
		{"group key", &FindCoordinatorRequest{Key: "analytics", KeyType: CoordinatorKeyTypeGroup}},
		{"transaction key", &FindCoordinatorRequest{Key: "txn-1", KeyType: CoordinatorKeyTypeTransaction}},
		{"empty key", &FindCoordinatorRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripRequest(t, RequestTypeFindCoordinator, tt.payload)
			if !reflect.DeepEqual(got, tt.payload) {
				t.Errorf("round trip changed the payload:\n got: %+v\nwant: %+v", got, tt.payload)
			}
		})
	}
}

func TestFindCoordinatorResponse_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload *FindCoordinatorResponse
	}{
		{"found", &FindCoordinatorResponse{NodeID: 2, Host: "broker-2", Port: 9092}},
		{"not coordinator", &FindCoordinatorResponse{ErrorCode: ErrNotCoordinator}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripResponse(t, RequestTypeFindCoordinator, tt.payload)
			if !reflect.DeepEqual(got, tt.payload) {
				t.Errorf("round trip changed the payload:\n got: %+v\nwant: %+v", got, tt.payload)
			}
		})
	}
}

func TestDecodeFindCoordinator_TruncatedPayload(t *testing.T) {
	// A truncated payload must error out rather than panic or return a
	// half-populated response.
	sizer := newSizer()
	full := &FindCoordinatorResponse{NodeID: 3, Host: "broker-3", Port: 9092}
	full.encodePayload(sizer)

	buf := make([]byte, sizer.Len())
	full.encodePayload(newWriter(buf, 0))

	for cut := 0; cut < len(buf); cut++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decoding a %d-byte prefix panicked: %v", cut, r)
				}
			}()
			_, err := decodeCoordinationResponse(buf[:cut], RequestTypeFindCoordinator)
			if err == nil {
				t.Errorf("decoding a %d-byte prefix (of %d) succeeded, want error", cut, len(buf))
			}
		}()
	}
}
