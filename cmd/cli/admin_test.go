package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gstreamio/streambus/pkg/broker"
)

// addrOf strips the http:// scheme off an httptest.Server URL, since
// newAdminClient takes a host:port address and prepends the scheme itself.
func addrOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	addr := strings.TrimPrefix(srv.URL, "http://")
	if addr == srv.URL {
		t.Fatalf("expected an http:// test server URL, got %q", srv.URL)
	}
	return addr
}

// TestAdminClient_ClusterInfo encodes broker.ClusterInfo itself
// (pkg/broker/admin_api.go's actual response type), rather than a
// hand-rolled fixture, so the test fails if the CLI's local struct ever
// drifts from the real response shape instead of quietly agreeing with a
// wrong assumption.
func TestAdminClient_ClusterInfo(t *testing.T) {
	want := broker.ClusterInfo{
		ClusterID:       "streambus-cluster-1",
		ControllerID:    1,
		Version:         "1.0.0",
		TotalBrokers:    3,
		ActiveBrokers:   2,
		TotalTopics:     5,
		TotalPartitions: 50,
		Uptime:          "1h2m3s",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/cluster" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	got, err := client.ClusterInfo(context.Background())
	if err != nil {
		t.Fatalf("ClusterInfo: %v", err)
	}

	if got.ClusterID != want.ClusterID ||
		got.ControllerID != want.ControllerID ||
		got.Version != want.Version ||
		got.TotalBrokers != want.TotalBrokers ||
		got.ActiveBrokers != want.ActiveBrokers ||
		got.TotalTopics != want.TotalTopics ||
		got.TotalPartitions != want.TotalPartitions ||
		got.Uptime != want.Uptime {
		t.Errorf("ClusterInfo() = %+v, want %+v", got, want)
	}
}

// TestAdminClient_Brokers exercises the GET /api/v1/brokers list shape
// against broker.BrokerInfo.
func TestAdminClient_Brokers(t *testing.T) {
	want := []broker.BrokerInfo{
		{ID: 1, Host: "broker-1", Port: 9092, Status: "alive", Leader: true, Version: "1.0.0", Uptime: "5m"},
		{ID: 2, Host: "broker-2", Port: 9092, Status: "alive", Leader: false, Version: "1.0.0", Uptime: "5m"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/brokers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	got, err := client.Brokers(context.Background())
	if err != nil {
		t.Fatalf("Brokers: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("Brokers() returned %d brokers, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Host != want[i].Host || got[i].Port != want[i].Port ||
			got[i].Status != want[i].Status || got[i].Leader != want[i].Leader {
			t.Errorf("Brokers()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestAdminClient_ConsumerGroups exercises the GET /api/v1/consumer-groups
// shape against broker.ConsumerGroupInfo, including the nested member list.
func TestAdminClient_ConsumerGroups(t *testing.T) {
	want := []broker.ConsumerGroupInfo{
		{
			GroupID:  "group-a",
			State:    "Stable",
			Protocol: "range",
			Members: []broker.MemberInfo{
				{MemberID: "member-1", ClientID: "client-1", ClientHost: "10.0.0.1", Partitions: []int32{0, 1}},
			},
			Coordinator: 1,
			TotalLag:    42,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/consumer-groups" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	got, err := client.ConsumerGroups(context.Background())
	if err != nil {
		t.Fatalf("ConsumerGroups: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ConsumerGroups() returned %d groups, want 1", len(got))
	}
	if got[0].GroupID != want[0].GroupID || got[0].State != want[0].State || len(got[0].Members) != 1 {
		t.Errorf("ConsumerGroups()[0] = %+v, want %+v", got[0], want[0])
	}
	if got[0].Members[0].MemberID != "member-1" {
		t.Errorf("ConsumerGroups()[0].Members[0].MemberID = %q, want %q", got[0].Members[0].MemberID, "member-1")
	}
}

// TestAdminClient_ErrorResponse covers the admin API's error path: handlers
// in pkg/broker/admin_api.go report failures via http.Error (plain text,
// non-2xx status), never as a 200 carrying a made-up zero value - so the
// client must surface that as an error, not decode it as an empty success.
func TestAdminClient_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "metadata store not available", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	_, err := client.ClusterInfo(context.Background())
	if err == nil {
		t.Fatal("expected an error for a non-2xx admin API response, got nil")
	}
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "metadata store not available") {
		t.Errorf("error = %q, want it to mention the status code and body", err.Error())
	}
}

// TestAdminClient_ConnectionFailure covers the case where the broker is
// simply unreachable (nothing listening on the given address) - this must
// fail the command, not silently report zero brokers/groups.
func TestAdminClient_ConnectionFailure(t *testing.T) {
	client := newAdminClient("127.0.0.1:1")
	_, err := client.ClusterInfo(context.Background())
	if err == nil {
		t.Fatal("expected an error when the admin API is unreachable, got nil")
	}
}
