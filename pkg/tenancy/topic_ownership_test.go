package tenancy

import "testing"

func TestRegisterTopic_TracksOwnership(t *testing.T) {
	m := NewManager()

	m.RegisterTopic("tenant-a", "orders")
	m.RegisterTopic("tenant-a", "shipments")
	m.RegisterTopic("tenant-b", "payments")

	owner, ok := m.TenantForTopic("orders")
	if !ok {
		t.Fatal("expected orders to have an owner")
	}
	if owner != "tenant-a" {
		t.Errorf("orders owner = %q, want tenant-a", owner)
	}

	topics := m.TopicsFor("tenant-a")
	if len(topics) != 2 || topics[0] != "orders" || topics[1] != "shipments" {
		t.Errorf("TopicsFor(tenant-a) = %v, want [orders shipments]", topics)
	}

	if got := m.TopicsFor("tenant-b"); len(got) != 1 || got[0] != "payments" {
		t.Errorf("TopicsFor(tenant-b) = %v, want [payments]", got)
	}
}

func TestTenantForTopic_UnknownTopic(t *testing.T) {
	m := NewManager()

	if _, ok := m.TenantForTopic("never-created"); ok {
		t.Error("expected no owner for an unregistered topic")
	}
}

func TestUnregisterTopic(t *testing.T) {
	m := NewManager()

	m.RegisterTopic("tenant-a", "orders")
	m.UnregisterTopic("orders")

	if _, ok := m.TenantForTopic("orders"); ok {
		t.Error("expected ownership to be dropped after UnregisterTopic")
	}
	if got := m.TopicsFor("tenant-a"); len(got) != 0 {
		t.Errorf("TopicsFor(tenant-a) = %v, want empty", got)
	}

	// Unregistering an unknown topic must not panic.
	m.UnregisterTopic("never-created")
}

func TestRegisterTopic_ReassignsOwner(t *testing.T) {
	m := NewManager()

	m.RegisterTopic("tenant-a", "orders")
	m.RegisterTopic("tenant-b", "orders")

	owner, _ := m.TenantForTopic("orders")
	if owner != "tenant-b" {
		t.Errorf("orders owner = %q, want tenant-b after re-registration", owner)
	}
	if got := m.TopicsFor("tenant-a"); len(got) != 0 {
		t.Errorf("tenant-a should no longer own orders, got %v", got)
	}
}

func TestRegisterTopic_IgnoresEmptyName(t *testing.T) {
	m := NewManager()

	m.RegisterTopic("tenant-a", "")

	if len(m.TopicOwners()) != 0 {
		t.Error("empty topic name should not be registered")
	}
}

func TestTopicOwners_ReturnsCopy(t *testing.T) {
	m := NewManager()
	m.RegisterTopic("tenant-a", "orders")

	owners := m.TopicOwners()
	owners["orders"] = "tenant-hacked"
	delete(owners, "orders")

	if owner, _ := m.TenantForTopic("orders"); owner != "tenant-a" {
		t.Errorf("mutating the returned map changed manager state: owner = %q", owner)
	}
}
