package tenancy

import "sort"

// RegisterTopic records that a topic belongs to a tenant.
//
// Ownership is what lets per-tenant storage accounting attribute on-disk bytes
// to the right tenant: without it, the broker knows how many bytes each topic
// occupies but not whose quota they count against. Registering the same topic
// again re-points it at the new owner, which is what a delete-then-recreate by
// a different tenant should do.
func (m *Manager) RegisterTopic(id TenantID, topic string) {
	if topic == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.topicOwners == nil {
		m.topicOwners = make(map[string]TenantID)
	}
	m.topicOwners[topic] = id
}

// UnregisterTopic drops a topic's ownership record. Unknown topics are ignored.
func (m *Manager) UnregisterTopic(topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.topicOwners, topic)
}

// TenantForTopic returns the tenant that owns a topic. The bool reports
// whether an ownership record exists; topics created before multi-tenancy was
// enabled have none.
func (m *Manager) TenantForTopic(topic string) (TenantID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.topicOwners[topic]
	return id, ok
}

// TopicsFor returns the topics owned by a tenant, sorted by name.
func (m *Manager) TopicsFor(id TenantID) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	topics := make([]string, 0)
	for topic, owner := range m.topicOwners {
		if owner == id {
			topics = append(topics, topic)
		}
	}

	sort.Strings(topics)
	return topics
}

// TopicOwners returns a copy of the full topic-to-tenant mapping.
func (m *Manager) TopicOwners() map[string]TenantID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	owners := make(map[string]TenantID, len(m.topicOwners))
	for topic, id := range m.topicOwners {
		owners[topic] = id
	}
	return owners
}
