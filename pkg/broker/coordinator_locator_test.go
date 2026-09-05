package broker

import (
	"context"
	"testing"

	"github.com/gstreamio/streambus/pkg/cluster"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// registerLiveBroker registers a broker and immediately marks it alive, since
// RegisterBroker itself only ever leaves a brand new broker in
// BrokerStatusStarting (see cluster.BrokerRegistry.RegisterBroker).
func registerLiveBroker(t *testing.T, registry *cluster.BrokerRegistry, id int32, host string, port int) {
	t.Helper()
	err := registry.RegisterBroker(context.Background(), &cluster.BrokerMetadata{
		ID:   id,
		Host: host,
		Port: port,
	})
	if err != nil {
		t.Fatalf("RegisterBroker(%d): %v", id, err)
	}
	if err := registry.RecordHeartbeat(id); err != nil {
		t.Fatalf("RecordHeartbeat(%d): %v", id, err)
	}
}

func TestRegistryCoordinatorLocator_NilRegistry(t *testing.T) {
	locator := newRegistryCoordinatorLocator(nil)

	_, _, _, errCode := locator.FindCoordinator(protocol.CoordinatorKeyTypeGroup, "analytics")
	if errCode != protocol.ErrNotCoordinator {
		t.Errorf("errCode = %v, want ErrNotCoordinator", errCode)
	}
}

func TestRegistryCoordinatorLocator_NoLiveBrokers(t *testing.T) {
	registry := cluster.NewBrokerRegistry(&mockClusterMetadataStore{})
	locator := newRegistryCoordinatorLocator(registry)

	nodeID, host, port, errCode := locator.FindCoordinator(protocol.CoordinatorKeyTypeGroup, "analytics")
	if errCode != protocol.ErrNotCoordinator {
		t.Errorf("errCode = %v, want ErrNotCoordinator", errCode)
	}
	if nodeID != 0 || host != "" || port != 0 {
		t.Errorf("got (%d, %q, %d), want zero values alongside the error", nodeID, host, port)
	}
}

func TestRegistryCoordinatorLocator_SingleBroker(t *testing.T) {
	registry := cluster.NewBrokerRegistry(&mockClusterMetadataStore{})
	registerLiveBroker(t, registry, 1, "broker-1", 9092)
	locator := newRegistryCoordinatorLocator(registry)

	for _, key := range []string{"analytics", "orders", "txn-42"} {
		nodeID, host, port, errCode := locator.FindCoordinator(protocol.CoordinatorKeyTypeGroup, key)
		if errCode != protocol.ErrNone {
			t.Fatalf("key %q: errCode = %v, want ErrNone", key, errCode)
		}
		if nodeID != 1 || host != "broker-1" || port != 9092 {
			t.Errorf("key %q: got (%d, %q, %d), want (1, broker-1, 9092)", key, nodeID, host, port)
		}
	}
}

func TestRegistryCoordinatorLocator_DeterministicAndStable(t *testing.T) {
	registry := cluster.NewBrokerRegistry(&mockClusterMetadataStore{})
	registerLiveBroker(t, registry, 1, "broker-1", 9092)
	registerLiveBroker(t, registry, 2, "broker-2", 9092)
	registerLiveBroker(t, registry, 3, "broker-3", 9092)

	// Two independent locators over the same registry state must agree -
	// this is what lets every broker in the cluster answer identically
	// without a coordination round trip.
	first := newRegistryCoordinatorLocator(registry)
	second := newRegistryCoordinatorLocator(registry)

	keys := []string{"analytics", "orders-group", "txn-1", "txn-2", "billing"}
	for _, key := range keys {
		id1, host1, port1, err1 := first.FindCoordinator(protocol.CoordinatorKeyTypeGroup, key)
		id2, host2, port2, err2 := second.FindCoordinator(protocol.CoordinatorKeyTypeGroup, key)
		if err1 != protocol.ErrNone || err2 != protocol.ErrNone {
			t.Fatalf("key %q: errCode = (%v, %v), want ErrNone", key, err1, err2)
		}
		if id1 != id2 || host1 != host2 || port1 != port2 {
			t.Errorf("key %q: two locators disagree: (%d,%q,%d) vs (%d,%q,%d)",
				key, id1, host1, port1, id2, host2, port2)
		}

		// Repeating the same call against the same locator must be stable.
		id3, _, _, _ := first.FindCoordinator(protocol.CoordinatorKeyTypeGroup, key)
		if id3 != id1 {
			t.Errorf("key %q: repeated call changed the answer: %d then %d", key, id1, id3)
		}
	}

	// Distinct keys should not all collapse onto the same broker - not a
	// hash-quality guarantee, just a sanity check that every broker is
	// reachable at all.
	seen := map[int32]bool{}
	for _, key := range keys {
		id, _, _, _ := first.FindCoordinator(protocol.CoordinatorKeyTypeGroup, key)
		seen[id] = true
	}
	if len(seen) < 2 {
		t.Errorf("all %d keys hashed to the same broker: %v", len(keys), seen)
	}
}

func TestRegistryCoordinatorLocator_StableAcrossRegistryIterationOrder(t *testing.T) {
	// ListActiveBrokers walks a map, so its order is not guaranteed between
	// calls; the locator must sort before hashing or two calls could answer
	// differently for the same live set.
	registry := cluster.NewBrokerRegistry(&mockClusterMetadataStore{})
	registerLiveBroker(t, registry, 5, "broker-5", 9092)
	registerLiveBroker(t, registry, 1, "broker-1", 9092)
	registerLiveBroker(t, registry, 3, "broker-3", 9092)
	locator := newRegistryCoordinatorLocator(registry)

	want, _, _, _ := locator.FindCoordinator(protocol.CoordinatorKeyTypeGroup, "analytics")
	for i := 0; i < 20; i++ {
		got, _, _, _ := locator.FindCoordinator(protocol.CoordinatorKeyTypeGroup, "analytics")
		if got != want {
			t.Fatalf("iteration %d: answer changed from %d to %d for the same live set", i, want, got)
		}
	}
}

func TestHashKey_StableAcrossBuilds(t *testing.T) {
	// Comparing hashKey(x) to hashKey(x) in the same process proves nothing:
	// it is a pure function, so that can never fail. What actually matters is
	// that the value is the same in every broker process and across releases,
	// since brokers must independently agree on who coordinates a key and a
	// rolling upgrade must not silently reshuffle every group. Pinning known
	// FNV-1a values is what catches the hash being swapped out.
	tests := []struct {
		key  string
		want uint32
	}{
		{"analytics", 0x5c10448b},
		{"orders", 0x325e96ec},
		{"", 0x811c9dc5}, // FNV-1a offset basis
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := hashKey(tt.key); got != tt.want {
				t.Errorf("hashKey(%q) = %#x, want %#x: changing this reassigns every "+
					"group and transactional ID to a different coordinator", tt.key, got, tt.want)
			}
		})
	}
}
