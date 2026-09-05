package group

import (
	"errors"
	"fmt"

	"github.com/gstreamio/streambus/pkg/logging"
)

// ErrGroupNotFound is returned by DeleteGroup when the group does not exist.
var ErrGroupNotFound = errors.New("consumer group not found")

// GroupNotEmptyError is returned by DeleteGroup when the group still has
// active members. Deleting such a group would discard the committed offsets
// those members still depend on, so it is refused rather than performed -
// mirroring Kafka's NON_EMPTY_GROUP behavior instead of silently yanking a
// live group out from under its consumers.
type GroupNotEmptyError struct {
	GroupID     string
	MemberCount int
}

func (e *GroupNotEmptyError) Error() string {
	return fmt.Sprintf("consumer group %q still has %d active member(s)", e.GroupID, e.MemberCount)
}

// ListGroups returns all consumer groups
func (gc *GroupCoordinator) ListGroups() []*GroupMetadata {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	groups := make([]*GroupMetadata, 0, len(gc.groups))
	for _, group := range gc.groups {
		// Make a copy to prevent external modifications
		groupCopy := *group
		groups = append(groups, &groupCopy)
	}

	return groups
}

// GetGroup returns a consumer group by ID
func (gc *GroupCoordinator) GetGroup(groupID string) *GroupMetadata {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	group, exists := gc.groups[groupID]
	if !exists {
		return nil
	}

	// Make a copy to prevent external modifications
	groupCopy := *group
	return &groupCopy
}

// GetCommittedOffset returns the committed offset for a group/topic/partition
func (gc *GroupCoordinator) GetCommittedOffset(groupID string, topic string, partition int32) (int64, error) {
	offsetMeta, err := gc.offsetStorage.FetchOffset(groupID, topic, partition)
	if err != nil {
		return 0, err
	}

	if offsetMeta == nil {
		return 0, nil
	}

	return offsetMeta.Offset, nil
}

// DeleteGroup removes a consumer group and its committed offsets.
//
// It returns ErrGroupNotFound for an unknown group, and a *GroupNotEmptyError
// if the group still has active members - deleting a live group would strip
// offsets out from under consumers still relying on them, so the caller must
// have those members leave (or expire) first.
//
// The group is unregistered from the coordinator's own map before its
// offsets are deleted, so a failure partway through never leaves a
// still-registered group pointing at offsets that are about to disappear;
// the reverse order could let a concurrent join see a live group that then
// loses its offsets underneath it.
func (gc *GroupCoordinator) DeleteGroup(groupID string) error {
	gc.mu.Lock()
	group, exists := gc.groups[groupID]
	if !exists {
		gc.mu.Unlock()
		return ErrGroupNotFound
	}
	if memberCount := len(group.Members); memberCount > 0 {
		gc.mu.Unlock()
		return &GroupNotEmptyError{GroupID: groupID, MemberCount: memberCount}
	}
	delete(gc.groups, groupID)
	gc.mu.Unlock()

	// Offsets live in a separate store with its own concurrency control, so
	// this runs outside gc.mu. A group removed while its offsets linger
	// would let a same-named group that joins later resurrect with stale
	// positions - deleting them is not optional cleanup.
	if err := gc.offsetStorage.DeleteOffsets(groupID); err != nil {
		return fmt.Errorf("group %q was removed but its committed offsets could not be deleted: %w", groupID, err)
	}

	gc.logger.Info("Deleted consumer group", logging.Fields{"group_id": groupID})
	return nil
}
