package broker

import (
	"hash/fnv"
	"sort"

	"github.com/gstreamio/streambus/pkg/cluster"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// registryCoordinatorLocator resolves FindCoordinator requests by hashing the
// key over the sorted set of currently live broker IDs. It satisfies
// server.CoordinatorLocator.
//
// This is a deterministic partitioning function rather than an assignment
// the cluster has to agree on and persist: every broker sees the same
// registry membership (modulo the same eventual-consistency window the
// registry already has) and computes the same answer from it, so there is
// nothing to coordinate for FindCoordinator itself - no Raft round trip, no
// leader election. A broker joining or leaving does reshuffle which key maps
// to which broker, exactly like a resized hash ring, which is the accepted
// tradeoff for not persisting a coordinator assignment anywhere.
type registryCoordinatorLocator struct {
	registry *cluster.BrokerRegistry
}

// newRegistryCoordinatorLocator creates a locator over registry. registry may
// be nil (a broker driven directly in a test, without initCluster), in which
// case FindCoordinator always reports ErrNotCoordinator.
func newRegistryCoordinatorLocator(registry *cluster.BrokerRegistry) *registryCoordinatorLocator {
	return &registryCoordinatorLocator{registry: registry}
}

// FindCoordinator implements server.CoordinatorLocator.
func (l *registryCoordinatorLocator) FindCoordinator(
	_ protocol.CoordinatorKeyType,
	key string,
) (nodeID int32, host string, port int32, errCode protocol.ErrorCode) {
	if l.registry == nil {
		return 0, "", 0, protocol.ErrNotCoordinator
	}

	brokers := l.registry.ListActiveBrokers()
	if len(brokers) == 0 {
		return 0, "", 0, protocol.ErrNotCoordinator
	}

	// Sorted so every broker's independent computation lands on the same
	// index for the same key, regardless of the map iteration order
	// ListActiveBrokers happened to return.
	sort.Slice(brokers, func(i, j int) bool { return brokers[i].ID < brokers[j].ID })

	chosen := brokers[hashKey(key)%uint32(len(brokers))]                // #nosec G115 -- len(brokers) > 0, checked above
	return chosen.ID, chosen.Host, int32(chosen.Port), protocol.ErrNone // #nosec G115 -- broker Port is always a valid TCP port, well within int32
}

// hashKey hashes a coordination key to a well-distributed uint32. FNV-1a is
// used purely as a stable, dependency-free hash - there is no need for
// cryptographic properties, only that every broker computes the same value
// for the same key.
func hashKey(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32()
}
