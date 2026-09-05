package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// adminRequestTimeout bounds a single admin API call so a hung or
// unreachable broker fails the command instead of hanging the CLI forever.
const adminRequestTimeout = 10 * time.Second

// adminClient talks to a single broker's admin HTTP API (registered by
// registerAdminAPI in pkg/broker/admin_api.go). Cluster, broker, and
// consumer-group information is only available there - it is not part of
// the native wire protocol that the client package (used by the topic and
// produce/consume commands) speaks.
type adminClient struct {
	baseURL    string
	httpClient *http.Client
}

// newAdminClient creates a client for the admin HTTP API at addr (host:port).
func newAdminClient(addr string) *adminClient {
	return &adminClient{
		baseURL:    "http://" + addr,
		httpClient: &http.Client{Timeout: adminRequestTimeout},
	}
}

// getJSON issues a GET to path and decodes the JSON response body into out.
// The admin API reports failures via http.Error (a plain-text body and a
// non-2xx status), so those are surfaced as an error rather than attempted
// as JSON - a broker that can't be reached, or that errors, must fail the
// command rather than leave out a plausible-looking zero value.
func (a *adminClient) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	resp, err := a.httpClient.Do(req) //nolint:gosec // baseURL comes from the operator's own --admin-addr flag, not untrusted input
	if err != nil {
		return fmt.Errorf("calling admin API at %s: %w", a.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding admin API response: %w", err)
	}
	return nil
}

// deleteJSON issues a DELETE to path. The admin API's delete endpoints
// respond with 204 No Content and no JSON body on success, so there is
// nothing to decode - only the status matters.
func (a *adminClient) deleteJSON(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	resp, err := a.httpClient.Do(req) //nolint:gosec // baseURL comes from the operator's own --admin-addr flag, not untrusted input
	if err != nil {
		return fmt.Errorf("calling admin API at %s: %w", a.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ClusterInfo mirrors the JSON shape of broker.ClusterInfo
// (pkg/broker/admin_api.go), returned by GET /api/v1/cluster.
type ClusterInfo struct {
	ClusterID       string `json:"cluster_id"`
	ControllerID    int32  `json:"controller_id"`
	Version         string `json:"version"`
	TotalBrokers    int    `json:"total_brokers"`
	ActiveBrokers   int    `json:"active_brokers"`
	TotalTopics     int    `json:"total_topics"`
	TotalPartitions int    `json:"total_partitions"`
	Uptime          string `json:"uptime"`
}

// ClusterInfo fetches cluster-wide metadata from GET /api/v1/cluster.
func (a *adminClient) ClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	var info ClusterInfo
	if err := a.getJSON(ctx, "/api/v1/cluster", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// BrokerInfo mirrors the JSON shape of broker.BrokerInfo, as returned by
// GET /api/v1/brokers (the Resources field is only populated by the
// per-broker detail endpoint, so it is omitted here).
type BrokerInfo struct {
	ID      int32  `json:"id"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Status  string `json:"status"`
	Leader  bool   `json:"leader"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// Brokers fetches the list of known brokers from GET /api/v1/brokers.
func (a *adminClient) Brokers(ctx context.Context) ([]BrokerInfo, error) {
	var brokers []BrokerInfo
	if err := a.getJSON(ctx, "/api/v1/brokers", &brokers); err != nil {
		return nil, err
	}
	return brokers, nil
}

// MemberInfo mirrors the JSON shape of broker.MemberInfo.
type MemberInfo struct {
	MemberID   string  `json:"member_id"`
	ClientID   string  `json:"client_id"`
	ClientHost string  `json:"client_host"`
	Partitions []int32 `json:"partitions"`
	JoinedAt   int64   `json:"joined_at"`
}

// ConsumerGroupInfo mirrors the JSON shape of broker.ConsumerGroupInfo, as
// returned by GET /api/v1/consumer-groups.
type ConsumerGroupInfo struct {
	GroupID     string       `json:"group_id"`
	State       string       `json:"state"`
	Protocol    string       `json:"protocol"`
	Members     []MemberInfo `json:"members"`
	Coordinator int32        `json:"coordinator"`
	TotalLag    int64        `json:"total_lag"`
}

// ConsumerGroups fetches all consumer groups from GET /api/v1/consumer-groups.
func (a *adminClient) ConsumerGroups(ctx context.Context) ([]ConsumerGroupInfo, error) {
	var groups []ConsumerGroupInfo
	if err := a.getJSON(ctx, "/api/v1/consumer-groups", &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// ConsumerGroup fetches a single consumer group's detail, including its
// members, from GET /api/v1/consumer-groups/:id.
func (a *adminClient) ConsumerGroup(ctx context.Context, groupID string) (*ConsumerGroupInfo, error) {
	var group ConsumerGroupInfo
	path := "/api/v1/consumer-groups/" + url.PathEscape(groupID)
	if err := a.getJSON(ctx, path, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// DeleteConsumerGroup deletes a consumer group via
// DELETE /api/v1/consumer-groups/:id. The admin API rejects a group that
// still has active members (409) and reports an unknown group as 404;
// deleteJSON surfaces both as an error whose text names which happened.
func (a *adminClient) DeleteConsumerGroup(ctx context.Context, groupID string) error {
	return a.deleteJSON(ctx, "/api/v1/consumer-groups/"+url.PathEscape(groupID))
}

// TopicInfo mirrors the JSON shape of broker.TopicResponse, as returned by
// GET /api/v1/topics/:name. Its Partitions field now carries real
// leader/replicas/ISR/offsets - pkg/broker/admin_api.go's getTopic and
// handleTopicPartitions both build it through the same buildPartitionInfos
// helper - so a single call here carries everything topic describe needs.
type TopicInfo struct {
	Name              string          `json:"name"`
	NumPartitions     int             `json:"num_partitions"`
	ReplicationFactor int             `json:"replication_factor"`
	Partitions        []PartitionInfo `json:"partitions,omitempty"`
}

// PartitionInfo mirrors the JSON shape of broker.PartitionInfo, as returned
// by GET /api/v1/topics/:name and GET /api/v1/topics/:name/partitions.
type PartitionInfo struct {
	ID              int32   `json:"id"`
	Leader          int32   `json:"leader"`
	Replicas        []int32 `json:"replicas"`
	ISR             []int32 `json:"isr"`
	BeginningOffset int64   `json:"beginning_offset"`
	EndOffset       int64   `json:"end_offset"`
	MessageCount    int64   `json:"message_count"`
}

// Topic fetches a single topic's metadata, including per-partition detail,
// from GET /api/v1/topics/:name.
func (a *adminClient) Topic(ctx context.Context, name string) (*TopicInfo, error) {
	var topic TopicInfo
	if err := a.getJSON(ctx, "/api/v1/topics/"+url.PathEscape(name), &topic); err != nil {
		return nil, err
	}
	return &topic, nil
}

// TopicPartitions fetches per-partition detail directly from
// GET /api/v1/topics/:name/partitions, without the rest of the topic's
// metadata. Topic above now returns the same partition detail alongside
// name/replication-factor, so this exists for callers that only want the
// partitions - it is not used by "topic describe" any more.
func (a *adminClient) TopicPartitions(ctx context.Context, name string) ([]PartitionInfo, error) {
	var partitions []PartitionInfo
	path := "/api/v1/topics/" + url.PathEscape(name) + "/partitions"
	if err := a.getJSON(ctx, path, &partitions); err != nil {
		return nil, err
	}
	return partitions, nil
}

// DeleteTopic deletes a topic via DELETE /api/v1/topics/:name.
func (a *adminClient) DeleteTopic(ctx context.Context, name string) error {
	return a.deleteJSON(ctx, "/api/v1/topics/"+url.PathEscape(name))
}
