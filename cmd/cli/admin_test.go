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

// TestAdminClient_ConsumerGroup exercises GET /api/v1/consumer-groups/:id
// against broker.ConsumerGroupInfo, the same real response type used by the
// list endpoint's test.
func TestAdminClient_ConsumerGroup(t *testing.T) {
	want := broker.ConsumerGroupInfo{
		GroupID:  "group-a",
		State:    "Stable",
		Protocol: "range",
		Members: []broker.MemberInfo{
			{MemberID: "member-1", ClientID: "client-1", ClientHost: "10.0.0.1", Partitions: []int32{0, 1}},
		},
		Coordinator: 1,
		TotalLag:    7,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/consumer-groups/group-a" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	got, err := client.ConsumerGroup(context.Background(), "group-a")
	if err != nil {
		t.Fatalf("ConsumerGroup: %v", err)
	}
	if got.GroupID != want.GroupID || got.State != want.State || got.Coordinator != want.Coordinator ||
		got.TotalLag != want.TotalLag || len(got.Members) != 1 {
		t.Errorf("ConsumerGroup() = %+v, want %+v", got, want)
	}
	if got.Members[0].ClientID != "client-1" || got.Members[0].ClientHost != "10.0.0.1" {
		t.Errorf("ConsumerGroup().Members[0] = %+v, want client_id/client_host from %+v", got.Members[0], want.Members[0])
	}
}

// TestAdminClient_ConsumerGroup_NotFound covers the admin API's 404 for an
// unknown group (pkg/broker/admin_api.go's getConsumerGroup).
func TestAdminClient_ConsumerGroup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Consumer group not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	_, err := client.ConsumerGroup(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for an unknown consumer group, got nil")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "Consumer group not found") {
		t.Errorf("error = %q, want it to mention the status code and body", err.Error())
	}
}

// TestAdminClient_Topic exercises GET /api/v1/topics/:name against
// broker.TopicResponse, including its embedded Partitions - since getTopic
// now builds those through the same buildPartitionInfos helper as the
// dedicated partitions endpoint, this response can carry real offsets too.
func TestAdminClient_Topic(t *testing.T) {
	want := broker.TopicResponse{
		Name:              "orders",
		NumPartitions:     1,
		ReplicationFactor: 3,
		Partitions: []broker.PartitionInfo{
			{ID: 0, Leader: 1, Replicas: []int32{1, 2, 3}, ISR: []int32{1, 2, 3}, BeginningOffset: 10, EndOffset: 110, MessageCount: 100},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/topics/orders" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	got, err := client.Topic(context.Background(), "orders")
	if err != nil {
		t.Fatalf("Topic: %v", err)
	}
	if got.Name != want.Name || got.NumPartitions != want.NumPartitions || got.ReplicationFactor != want.ReplicationFactor {
		t.Errorf("Topic() = %+v, want %+v", got, want)
	}
	if len(got.Partitions) != 1 {
		t.Fatalf("Topic().Partitions returned %d partitions, want 1", len(got.Partitions))
	}
	if got.Partitions[0].EndOffset != 110 || got.Partitions[0].MessageCount != 100 {
		t.Errorf("Topic().Partitions[0] = %+v, want real offsets from %+v", got.Partitions[0], want.Partitions[0])
	}
}

// TestAdminClient_Topic_NotFound covers the admin API's 404 for an unknown
// topic (pkg/broker/admin_api.go's getTopic).
func TestAdminClient_Topic_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Topic not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	_, err := client.Topic(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for an unknown topic, got nil")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "Topic not found") {
		t.Errorf("error = %q, want it to mention the status code and body", err.Error())
	}
}

// TestAdminClient_TopicPartitions exercises
// GET /api/v1/topics/:name/partitions against broker.PartitionInfo,
// including the real offsets that endpoint (unlike GET /api/v1/topics/:name)
// actually reads from storage.
func TestAdminClient_TopicPartitions(t *testing.T) {
	want := []broker.PartitionInfo{
		{ID: 0, Leader: 1, Replicas: []int32{1, 2, 3}, ISR: []int32{1, 2, 3}, BeginningOffset: 10, EndOffset: 110, MessageCount: 100},
		{ID: 1, Leader: 2, Replicas: []int32{2, 3, 1}, ISR: []int32{2, 3, 1}, BeginningOffset: 0, EndOffset: 5, MessageCount: 5},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/topics/orders/partitions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	got, err := client.TopicPartitions(context.Background(), "orders")
	if err != nil {
		t.Fatalf("TopicPartitions: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("TopicPartitions() returned %d partitions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Leader != want[i].Leader ||
			got[i].BeginningOffset != want[i].BeginningOffset || got[i].EndOffset != want[i].EndOffset ||
			got[i].MessageCount != want[i].MessageCount {
			t.Errorf("TopicPartitions()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestAdminClient_DeleteTopic_Success covers the 204 No Content success path
// (pkg/broker/admin_api.go's deleteTopic) - no JSON body to decode.
func TestAdminClient_DeleteTopic_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	if err := client.DeleteTopic(context.Background(), "orders"); err != nil {
		t.Fatalf("DeleteTopic: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/topics/orders" {
		t.Errorf("path = %q, want /api/v1/topics/orders", gotPath)
	}
}

// TestAdminClient_DeleteTopic_Error covers a non-2xx response, asserting the
// status and body both reach the returned error.
func TestAdminClient_DeleteTopic_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed to delete topic: still has active producers", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	err := client.DeleteTopic(context.Background(), "orders")
	if err == nil {
		t.Fatal("expected an error for a non-2xx DeleteTopic response, got nil")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "still has active producers") {
		t.Errorf("error = %q, want it to mention the status code and body", err.Error())
	}
}

// TestAdminClient_DeleteTopic_ConnectionFailure covers an unreachable broker
// - this must fail the command, never silently report success.
func TestAdminClient_DeleteTopic_ConnectionFailure(t *testing.T) {
	client := newAdminClient("127.0.0.1:1")
	if err := client.DeleteTopic(context.Background(), "orders"); err == nil {
		t.Fatal("expected an error when the admin API is unreachable, got nil")
	}
}

// TestAdminClient_DeleteConsumerGroup_Success covers the 204 No Content
// success path (pkg/broker/admin_api.go's deleteConsumerGroup).
func TestAdminClient_DeleteConsumerGroup_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	if err := client.DeleteConsumerGroup(context.Background(), "my-group"); err != nil {
		t.Fatalf("DeleteConsumerGroup: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/consumer-groups/my-group" {
		t.Errorf("path = %q, want /api/v1/consumer-groups/my-group", gotPath)
	}
}

// TestAdminClient_DeleteConsumerGroup_NonEmptyRejected covers the 409 the
// admin API returns for a group that still has active members - the error
// must surface the status and the member-count message, not be swallowed.
func TestAdminClient_DeleteConsumerGroup_NonEmptyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `consumer group "my-group" still has 2 active member(s)`, http.StatusConflict)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	err := client.DeleteConsumerGroup(context.Background(), "my-group")
	if err == nil {
		t.Fatal("expected an error for a non-empty group, got nil")
	}
	if !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "still has 2 active member(s)") {
		t.Errorf("error = %q, want it to mention the status code and member count", err.Error())
	}
}

// TestAdminClient_DeleteConsumerGroup_NotFound covers the 404 for an unknown
// group.
func TestAdminClient_DeleteConsumerGroup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "consumer group not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := newAdminClient(addrOf(t, srv))
	err := client.DeleteConsumerGroup(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for an unknown group, got nil")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "consumer group not found") {
		t.Errorf("error = %q, want it to mention the status code and body", err.Error())
	}
}

// TestAdminClient_DeleteConsumerGroup_ConnectionFailure covers an
// unreachable broker - this must fail the command, never silently report
// success.
func TestAdminClient_DeleteConsumerGroup_ConnectionFailure(t *testing.T) {
	client := newAdminClient("127.0.0.1:1")
	if err := client.DeleteConsumerGroup(context.Background(), "my-group"); err == nil {
		t.Fatal("expected an error when the admin API is unreachable, got nil")
	}
}
