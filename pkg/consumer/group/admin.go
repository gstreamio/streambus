package group

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
