package group

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Range Assignor Tests ---

func TestRangeAssignor_Name(t *testing.T) {
	a := &RangeAssignor{}
	assert.Equal(t, "range", a.Name())
}

func TestRangeAssignor_Assign(t *testing.T) {
	tests := []struct {
		name       string
		members    []MemberSubscription
		partitions []TopicPartition
		wantCounts map[string]int // memberID -> expected partition count
		validate   func(t *testing.T, result map[string][]TopicPartition)
	}{
		{
			name:       "empty members",
			members:    []MemberSubscription{},
			partitions: []TopicPartition{{Topic: "t1", Partition: 0}},
			wantCounts: map[string]int{},
		},
		{
			name:       "empty partitions",
			members:    []MemberSubscription{{MemberID: "m1", Topics: []string{"t1"}}},
			partitions: []TopicPartition{},
			wantCounts: map[string]int{"m1": 0},
		},
		{
			name: "single member single partition",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
			},
			wantCounts: map[string]int{"m1": 1},
		},
		{
			name: "single member multiple partitions",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
			},
			wantCounts: map[string]int{"m1": 3},
		},
		{
			name: "even distribution two members",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 2},
		},
		{
			name: "uneven distribution three partitions two members",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 1},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// First member (alphabetically) gets extra partition
				assert.Equal(t, int32(0), result["m1"][0].Partition)
				assert.Equal(t, int32(1), result["m1"][1].Partition)
				assert.Equal(t, int32(2), result["m2"][0].Partition)
			},
		},
		{
			name: "multiple topics different subscriptions",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1", "t2"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t2", Partition: 0},
				{Topic: "t2", Partition: 1},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// t1: both subscribe -> 1 each
				// t2: only m1 subscribes -> 2 for m1
				m1Topics := topicSet(result["m1"])
				assert.True(t, m1Topics["t1"])
				assert.True(t, m1Topics["t2"])

				m2Topics := topicSet(result["m2"])
				assert.True(t, m2Topics["t1"])
				assert.False(t, m2Topics["t2"])
			},
		},
		{
			name: "three members five partitions",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
				{MemberID: "m3", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
				{Topic: "t1", Partition: 4},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 2, "m3": 1},
		},
		{
			name: "more members than partitions",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
				{MemberID: "m3", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
			},
			wantCounts: map[string]int{"m1": 1, "m2": 1, "m3": 0},
		},
		{
			name: "no subscriber for topic",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t2", Partition: 0},
			},
			wantCounts: map[string]int{"m1": 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &RangeAssignor{}
			result := a.Assign(tt.members, tt.partitions)

			if tt.wantCounts != nil {
				for memberID, count := range tt.wantCounts {
					assert.Len(t, result[memberID], count, "member %s", memberID)
				}
			}
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// --- Round-Robin Assignor Tests ---

func TestRoundRobinAssignor_Name(t *testing.T) {
	a := &RoundRobinAssignor{}
	assert.Equal(t, "roundrobin", a.Name())
}

func TestRoundRobinAssignor_Assign(t *testing.T) {
	tests := []struct {
		name       string
		members    []MemberSubscription
		partitions []TopicPartition
		wantCounts map[string]int
		validate   func(t *testing.T, result map[string][]TopicPartition)
	}{
		{
			name:       "empty members",
			members:    []MemberSubscription{},
			partitions: []TopicPartition{{Topic: "t1", Partition: 0}},
			wantCounts: map[string]int{},
		},
		{
			name:       "empty partitions",
			members:    []MemberSubscription{{MemberID: "m1", Topics: []string{"t1"}}},
			partitions: []TopicPartition{},
			wantCounts: map[string]int{"m1": 0},
		},
		{
			name: "single member gets all",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
			},
			wantCounts: map[string]int{"m1": 3},
		},
		{
			name: "even distribution round-robin",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 2},
		},
		{
			name: "uneven distribution round-robin",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 1},
		},
		{
			name: "cross-topic round-robin",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1", "t2"}},
				{MemberID: "m2", Topics: []string{"t1", "t2"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t2", Partition: 0},
				{Topic: "t2", Partition: 1},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 2},
		},
		{
			name: "partial topic subscription",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1", "t2"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t2", Partition: 0},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// t2 partitions can only go to m1
				for _, tp := range result["m2"] {
					assert.NotEqual(t, "t2", tp.Topic)
				}
				t2Found := false
				for _, tp := range result["m1"] {
					if tp.Topic == "t2" {
						t2Found = true
					}
				}
				assert.True(t, t2Found, "m1 should have t2 partition")
			},
		},
		{
			name: "three members round-robin",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
				{MemberID: "m3", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
				{Topic: "t1", Partition: 4},
				{Topic: "t1", Partition: 5},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 2, "m3": 2},
		},
		{
			name: "more members than partitions",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
				{MemberID: "m3", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				total := 0
				for _, tps := range result {
					total += len(tps)
				}
				assert.Equal(t, 1, total)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &RoundRobinAssignor{}
			result := a.Assign(tt.members, tt.partitions)

			if tt.wantCounts != nil {
				for memberID, count := range tt.wantCounts {
					assert.Len(t, result[memberID], count, "member %s", memberID)
				}
			}
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// --- Sticky Assignor Tests ---

func TestStickyAssignor_Name(t *testing.T) {
	a := &StickyAssignor{}
	assert.Equal(t, "sticky", a.Name())
}

func TestStickyAssignor_Assign(t *testing.T) {
	tests := []struct {
		name       string
		members    []MemberSubscription
		partitions []TopicPartition
		wantCounts map[string]int
	}{
		{
			name:       "empty members",
			members:    []MemberSubscription{},
			partitions: []TopicPartition{{Topic: "t1", Partition: 0}},
			wantCounts: map[string]int{},
		},
		{
			name:       "empty partitions",
			members:    []MemberSubscription{{MemberID: "m1", Topics: []string{"t1"}}},
			partitions: []TopicPartition{},
			wantCounts: map[string]int{"m1": 0},
		},
		{
			name: "initial assignment uses round-robin fallback",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
			},
			wantCounts: map[string]int{"m1": 2, "m2": 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &StickyAssignor{}
			result := a.Assign(tt.members, tt.partitions)

			if tt.wantCounts != nil {
				for memberID, count := range tt.wantCounts {
					assert.Len(t, result[memberID], count, "member %s", memberID)
				}
			}
		})
	}
}

func TestStickyAssignor_AssignWithPrevious(t *testing.T) {
	tests := []struct {
		name       string
		members    []MemberSubscription
		partitions []TopicPartition
		previous   map[string][]TopicPartition
		validate   func(t *testing.T, result map[string][]TopicPartition)
	}{
		{
			name: "preserves assignments after new member joins",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
				{MemberID: "m3", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
				{Topic: "t1", Partition: 4},
				{Topic: "t1", Partition: 5},
			},
			previous: map[string][]TopicPartition{
				"m1": {{Topic: "t1", Partition: 0}, {Topic: "t1", Partition: 1}, {Topic: "t1", Partition: 2}},
				"m2": {{Topic: "t1", Partition: 3}, {Topic: "t1", Partition: 4}, {Topic: "t1", Partition: 5}},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// All 6 partitions should be assigned
				total := countTotal(result)
				assert.Equal(t, 6, total)
				// m3 should get some partitions
				assert.True(t, len(result["m3"]) > 0, "new member should get partitions")
				// Existing assignments should be preserved as much as possible
				// m1 and m2 should retain some of their original partitions
				assert.True(t, len(result["m1"]) > 0, "m1 should retain some partitions")
				assert.True(t, len(result["m2"]) > 0, "m2 should retain some partitions")
			},
		},
		{
			name: "handles member departure",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				// m2 has left
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
			},
			previous: map[string][]TopicPartition{
				"m1": {{Topic: "t1", Partition: 0}, {Topic: "t1", Partition: 1}},
				"m2": {{Topic: "t1", Partition: 2}, {Topic: "t1", Partition: 3}},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// m1 gets all 4 partitions (m2 left)
				assert.Len(t, result["m1"], 4)
				// m1's original partitions (0,1) should still be there
				partSet := partitionSet(result["m1"])
				assert.True(t, partSet[0])
				assert.True(t, partSet[1])
				assert.True(t, partSet[2])
				assert.True(t, partSet[3])
			},
		},
		{
			name: "preserves when no change in membership",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2},
				{Topic: "t1", Partition: 3},
			},
			previous: map[string][]TopicPartition{
				"m1": {{Topic: "t1", Partition: 0}, {Topic: "t1", Partition: 1}},
				"m2": {{Topic: "t1", Partition: 2}, {Topic: "t1", Partition: 3}},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// Assignments should be identical to previous
				assert.Len(t, result["m1"], 2)
				assert.Len(t, result["m2"], 2)
				m1Parts := partitionSet(result["m1"])
				assert.True(t, m1Parts[0])
				assert.True(t, m1Parts[1])
				m2Parts := partitionSet(result["m2"])
				assert.True(t, m2Parts[2])
				assert.True(t, m2Parts[3])
			},
		},
		{
			name: "drops previous for unsubscribed topic",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}}, // m1 no longer subscribes to t2
				{MemberID: "m2", Topics: []string{"t1", "t2"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t2", Partition: 0},
			},
			previous: map[string][]TopicPartition{
				"m1": {{Topic: "t1", Partition: 0}, {Topic: "t2", Partition: 0}},
				"m2": {},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// m1 retains t1:0 but loses t2:0
				m1Topics := topicSet(result["m1"])
				assert.True(t, m1Topics["t1"])
				assert.False(t, m1Topics["t2"])
				// m2 gets t2:0
				m2Topics := topicSet(result["m2"])
				assert.True(t, m2Topics["t2"])
			},
		},
		{
			name: "new partitions added",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
				{Topic: "t1", Partition: 2}, // new
				{Topic: "t1", Partition: 3}, // new
			},
			previous: map[string][]TopicPartition{
				"m1": {{Topic: "t1", Partition: 0}},
				"m2": {{Topic: "t1", Partition: 1}},
			},
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				// Original assignments preserved
				assert.Len(t, result["m1"], 2)
				assert.Len(t, result["m2"], 2)
				m1Parts := partitionSet(result["m1"])
				assert.True(t, m1Parts[0], "m1 should retain partition 0")
				m2Parts := partitionSet(result["m2"])
				assert.True(t, m2Parts[1], "m2 should retain partition 1")
			},
		},
		{
			name: "nil previous falls back to round-robin",
			members: []MemberSubscription{
				{MemberID: "m1", Topics: []string{"t1"}},
				{MemberID: "m2", Topics: []string{"t1"}},
			},
			partitions: []TopicPartition{
				{Topic: "t1", Partition: 0},
				{Topic: "t1", Partition: 1},
			},
			previous: nil,
			validate: func(t *testing.T, result map[string][]TopicPartition) {
				assert.Len(t, result["m1"], 1)
				assert.Len(t, result["m2"], 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &StickyAssignor{}
			result := a.AssignWithPrevious(tt.members, tt.partitions, tt.previous)
			tt.validate(t, result)
		})
	}
}

// --- GetAssignor Tests ---

func TestGetAssignor(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{name: "range", expected: "range"},
		{name: "roundrobin", expected: "roundrobin"},
		{name: "sticky", expected: "sticky"},
		{name: "unknown", expected: "range"},
		{name: "", expected: "range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := GetAssignor(tt.name)
			require.NotNil(t, a)
			assert.Equal(t, tt.expected, a.Name())
		})
	}
}

// --- Interface compliance ---

func TestAssignorInterface(t *testing.T) {
	var _ PartitionAssignor = &RangeAssignor{}
	var _ PartitionAssignor = &RoundRobinAssignor{}
	var _ PartitionAssignor = &StickyAssignor{}
}

// --- Helpers ---

func TestHelpers_groupPartitionsByTopic(t *testing.T) {
	partitions := []TopicPartition{
		{Topic: "t1", Partition: 0},
		{Topic: "t2", Partition: 0},
		{Topic: "t1", Partition: 1},
	}
	grouped := groupPartitionsByTopic(partitions)
	assert.Len(t, grouped["t1"], 2)
	assert.Len(t, grouped["t2"], 1)
}

func TestHelpers_subscribersForTopic(t *testing.T) {
	members := []MemberSubscription{
		{MemberID: "m1", Topics: []string{"t1", "t2"}},
		{MemberID: "m2", Topics: []string{"t1"}},
		{MemberID: "m3", Topics: []string{"t2"}},
	}
	subs := subscribersForTopic(members, "t1")
	assert.Len(t, subs, 2)
	assert.Contains(t, subs, "m1")
	assert.Contains(t, subs, "m2")

	subs = subscribersForTopic(members, "t3")
	assert.Empty(t, subs)
}

func TestHelpers_sortTopicPartitions(t *testing.T) {
	partitions := []TopicPartition{
		{Topic: "t2", Partition: 1},
		{Topic: "t1", Partition: 2},
		{Topic: "t1", Partition: 0},
		{Topic: "t2", Partition: 0},
	}
	sortTopicPartitions(partitions)
	assert.Equal(t, "t1", partitions[0].Topic)
	assert.Equal(t, int32(0), partitions[0].Partition)
	assert.Equal(t, "t1", partitions[1].Topic)
	assert.Equal(t, int32(2), partitions[1].Partition)
	assert.Equal(t, "t2", partitions[2].Topic)
	assert.Equal(t, int32(0), partitions[2].Partition)
	assert.Equal(t, "t2", partitions[3].Topic)
	assert.Equal(t, int32(1), partitions[3].Partition)
}

func TestHelpers_collectUnassigned(t *testing.T) {
	all := []TopicPartition{
		{Topic: "t1", Partition: 0},
		{Topic: "t1", Partition: 1},
		{Topic: "t1", Partition: 2},
	}
	assigned := map[TopicPartition]bool{
		{Topic: "t1", Partition: 0}: true,
	}
	unassigned := collectUnassigned(all, assigned)
	assert.Len(t, unassigned, 2)
}

// --- All partitions assigned validation ---

func TestAllPartitionsAssigned(t *testing.T) {
	assignors := []PartitionAssignor{
		&RangeAssignor{},
		&RoundRobinAssignor{},
		&StickyAssignor{},
	}

	members := []MemberSubscription{
		{MemberID: "m1", Topics: []string{"t1", "t2"}},
		{MemberID: "m2", Topics: []string{"t1", "t2"}},
		{MemberID: "m3", Topics: []string{"t1", "t2"}},
	}

	partitions := []TopicPartition{
		{Topic: "t1", Partition: 0},
		{Topic: "t1", Partition: 1},
		{Topic: "t1", Partition: 2},
		{Topic: "t2", Partition: 0},
		{Topic: "t2", Partition: 1},
	}

	for _, a := range assignors {
		t.Run(a.Name(), func(t *testing.T) {
			result := a.Assign(members, partitions)
			total := countTotal(result)
			assert.Equal(t, len(partitions), total, "all partitions should be assigned")

			// No duplicates
			seen := make(map[TopicPartition]bool)
			for _, tps := range result {
				for _, tp := range tps {
					assert.False(t, seen[tp], "partition %v assigned twice", tp)
					seen[tp] = true
				}
			}
		})
	}
}

// test helper functions

func topicSet(tps []TopicPartition) map[string]bool {
	s := make(map[string]bool)
	for _, tp := range tps {
		s[tp.Topic] = true
	}
	return s
}

func partitionSet(tps []TopicPartition) map[int32]bool {
	s := make(map[int32]bool)
	for _, tp := range tps {
		s[tp.Partition] = true
	}
	return s
}

func countTotal(result map[string][]TopicPartition) int {
	total := 0
	for _, tps := range result {
		total += len(tps)
	}
	return total
}
