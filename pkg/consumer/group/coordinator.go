package group

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawntherrien/streambus/pkg/logging"
)

// GroupCoordinator manages consumer groups
type GroupCoordinator struct {
	mu     sync.RWMutex
	groups map[string]*GroupMetadata
	logger *logging.Logger

	// Offset storage
	offsetStorage OffsetStorage

	// Configuration
	config CoordinatorConfig

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CoordinatorConfig holds coordinator configuration
type CoordinatorConfig struct {
	// Default session timeout
	DefaultSessionTimeoutMs int32

	// Default rebalance timeout
	DefaultRebalanceTimeoutMs int32

	// Min session timeout
	MinSessionTimeoutMs int32

	// Max session timeout
	MaxSessionTimeoutMs int32

	// Heartbeat check interval
	HeartbeatCheckIntervalMs int32

	// Offset retention time
	OffsetRetentionMs int64
}

// DefaultCoordinatorConfig returns default configuration
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		DefaultSessionTimeoutMs:   30000, // 30 seconds
		DefaultRebalanceTimeoutMs: 60000, // 60 seconds
		MinSessionTimeoutMs:       6000,  // 6 seconds
		MaxSessionTimeoutMs:       300000, // 5 minutes
		HeartbeatCheckIntervalMs:  3000,  // 3 seconds
		OffsetRetentionMs:         86400000, // 24 hours
	}
}

// OffsetStorage interface for storing offsets
type OffsetStorage interface {
	// StoreOffset stores an offset for a group/topic/partition
	StoreOffset(groupID string, topic string, partition int32, offset *OffsetAndMetadata) error

	// FetchOffset fetches an offset for a group/topic/partition
	FetchOffset(groupID string, topic string, partition int32) (*OffsetAndMetadata, error)

	// FetchOffsets fetches all offsets for a group
	FetchOffsets(groupID string) (*GroupOffsets, error)

	// DeleteOffsets deletes offsets for a group
	DeleteOffsets(groupID string) error
}

// NewGroupCoordinator creates a new group coordinator
func NewGroupCoordinator(storage OffsetStorage, config CoordinatorConfig) *GroupCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	logger := logging.New(&logging.Config{
		Level:     logging.LevelInfo,
		Component: "group-coordinator",
	})

	gc := &GroupCoordinator{
		groups:        make(map[string]*GroupMetadata),
		offsetStorage: storage,
		config:        config,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start background tasks
	gc.wg.Add(1)
	go gc.heartbeatChecker()

	return gc
}

// Stop stops the coordinator
func (gc *GroupCoordinator) Stop() error {
	gc.cancel()
	gc.wg.Wait()
	return nil
}

// HandleJoinGroup handles a join group request
func (gc *GroupCoordinator) HandleJoinGroup(req *JoinGroupRequest) (*JoinGroupResponse, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	// Validate request
	if err := gc.validateJoinRequest(req); err != nil {
		return &JoinGroupResponse{
			ErrorCode: err.Code,
		}, nil
	}

	// Get or create group
	group := gc.getOrCreateGroup(req.GroupID, req.ProtocolType)

	// Generate member ID if not provided
	memberID := req.MemberID
	if memberID == "" {
		memberID = gc.generateMemberID(req.ClientID)
	}

	// Add or update member
	member := &MemberMetadata{
		MemberID:           memberID,
		ClientID:           req.ClientID,
		State:              MemberStateJoining,
		SessionTimeoutMs:   req.SessionTimeoutMs,
		RebalanceTimeoutMs: req.RebalanceTimeoutMs,
		LastHeartbeat:      time.Now(),
		JoinTime:           time.Now(),
	}

	// Extract subscription from protocols
	if len(req.Protocols) > 0 {
		member.ProtocolMetadata = req.Protocols[0].Metadata
		// TODO: Parse subscription from metadata
	}

	group.Members[memberID] = member

	// Transition group state based on current state
	switch group.State {
	case GroupStateEmpty:
		// First member joining
		group.State = GroupStatePreparingRebalance
		group.LeaderID = memberID
		group.GenerationID++
		group.StateTimestamp = time.Now()

		gc.logger.Info("Group transitioning to PreparingRebalance", logging.Fields{
			"group_id":      group.GroupID,
			"member_id":     memberID,
			"generation_id": group.GenerationID,
		})

	case GroupStateStable:
		// New member joining stable group, trigger rebalance
		group.State = GroupStatePreparingRebalance
		group.GenerationID++
		group.StateTimestamp = time.Now()

		gc.logger.Info("Group rebalancing due to new member", logging.Fields{
			"group_id":      group.GroupID,
			"member_id":     memberID,
			"generation_id": group.GenerationID,
		})

	case GroupStatePreparingRebalance:
		// Already rebalancing, check if we need to bump generation
		// (This handles concurrent joins during rebalancing)
		gc.logger.Info("Member joining during rebalance", logging.Fields{
			"group_id":  group.GroupID,
			"member_id": memberID,
		})
	}

	// Select protocol (use first protocol from first member for now)
	if group.ProtocolName == "" && len(req.Protocols) > 0 {
		group.ProtocolName = req.Protocols[0].Name
	}

	// Prepare response
	resp := &JoinGroupResponse{
		ErrorCode:    ErrorCodeNone,
		GenerationID: group.GenerationID,
		ProtocolName: group.ProtocolName,
		MemberID:     memberID,
		LeaderID:     group.LeaderID,
	}

	// If this is the leader, include all members
	if memberID == group.LeaderID {
		for mid, m := range group.Members {
			resp.Members = append(resp.Members, JoinGroupMember{
				MemberID: mid,
				Metadata: m.ProtocolMetadata,
			})
		}
	}

	return resp, nil
}

// HandleSyncGroup handles a sync group request
func (gc *GroupCoordinator) HandleSyncGroup(req *SyncGroupRequest) (*SyncGroupResponse, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	group, err := gc.getGroup(req.GroupID)
	if err != nil {
		return &SyncGroupResponse{
			ErrorCode: err.Code,
		}, nil
	}

	// Validate generation
	if group.GenerationID != req.GenerationID {
		return &SyncGroupResponse{
			ErrorCode: ErrorCodeIllegalGeneration,
		}, nil
	}

	// Validate member
	member, exists := group.Members[req.MemberID]
	if !exists {
		return &SyncGroupResponse{
			ErrorCode: ErrorCodeUnknownMemberID,
		}, nil
	}

	// If leader, store assignments
	if req.MemberID == group.LeaderID && len(req.Assignments) > 0 {
		for _, assignment := range req.Assignments {
			if m, ok := group.Members[assignment.MemberID]; ok {
				// TODO: Parse assignment bytes into MemberAssignment struct
				m.State = MemberStateStable
			}
		}

		// Transition group to stable
		group.State = GroupStateStable
		group.StateTimestamp = time.Now()

		gc.logger.Info("Group transitioned to Stable", logging.Fields{
			"group_id":      group.GroupID,
			"generation_id": group.GenerationID,
			"members":       len(group.Members),
		})
	}

	// Update member state
	member.State = MemberStateStable

	// Return assignment for this member
	var assignment []byte
	for _, a := range req.Assignments {
		if a.MemberID == req.MemberID {
			assignment = a.Assignment
			break
		}
	}

	return &SyncGroupResponse{
		ErrorCode:  ErrorCodeNone,
		Assignment: assignment,
	}, nil
}

// HandleHeartbeat handles a heartbeat request
func (gc *GroupCoordinator) HandleHeartbeat(req *HeartbeatRequest) (*HeartbeatResponse, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	group, err := gc.getGroup(req.GroupID)
	if err != nil {
		return &HeartbeatResponse{
			ErrorCode: err.Code,
		}, nil
	}

	// Validate generation
	if group.GenerationID != req.GenerationID {
		return &HeartbeatResponse{
			ErrorCode: ErrorCodeIllegalGeneration,
		}, nil
	}

	// Validate and update member
	member, exists := group.Members[req.MemberID]
	if !exists {
		return &HeartbeatResponse{
			ErrorCode: ErrorCodeUnknownMemberID,
		}, nil
	}

	// Check if rebalance in progress
	if group.State == GroupStatePreparingRebalance {
		return &HeartbeatResponse{
			ErrorCode: ErrorCodeRebalanceInProgress,
		}, nil
	}

	// Update heartbeat timestamp
	member.LastHeartbeat = time.Now()

	return &HeartbeatResponse{
		ErrorCode: ErrorCodeNone,
	}, nil
}

// HandleLeaveGroup handles a leave group request
func (gc *GroupCoordinator) HandleLeaveGroup(req *LeaveGroupRequest) (*LeaveGroupResponse, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	group, err := gc.getGroup(req.GroupID)
	if err != nil {
		return &LeaveGroupResponse{
			ErrorCode: err.Code,
		}, nil
	}

	// Remove member
	delete(group.Members, req.MemberID)

	gc.logger.Info("Member left group", logging.Fields{
		"group_id":  req.GroupID,
		"member_id": req.MemberID,
	})

	// Handle state transitions
	if len(group.Members) == 0 {
		// Group is now empty
		group.State = GroupStateEmpty
		gc.logger.Info("Group is now empty", logging.Fields{
			"group_id": req.GroupID,
		})
	} else if group.State == GroupStateStable {
		// Trigger rebalance
		group.State = GroupStatePreparingRebalance
		group.GenerationID++
		group.StateTimestamp = time.Now()

		gc.logger.Info("Group rebalancing due to member departure", logging.Fields{
			"group_id":      req.GroupID,
			"generation_id": group.GenerationID,
		})
	}

	return &LeaveGroupResponse{
		ErrorCode: ErrorCodeNone,
	}, nil
}

// HandleOffsetCommit handles an offset commit request
func (gc *GroupCoordinator) HandleOffsetCommit(req *OffsetCommitRequest) (*OffsetCommitResponse, error) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	resp := &OffsetCommitResponse{
		Errors: make(map[string]map[int32]int16),
	}

	// Validate group if generation is specified
	if req.GenerationID >= 0 {
		group, err := gc.getGroup(req.GroupID)
		if err != nil {
			// Return error for all topics/partitions
			for topic, partitions := range req.Offsets {
				resp.Errors[topic] = make(map[int32]int16)
				for partition := range partitions {
					resp.Errors[topic][partition] = err.Code
				}
			}
			return resp, nil
		}

		// Validate member
		if _, exists := group.Members[req.MemberID]; !exists {
			for topic, partitions := range req.Offsets {
				resp.Errors[topic] = make(map[int32]int16)
				for partition := range partitions {
					resp.Errors[topic][partition] = ErrorCodeUnknownMemberID
				}
			}
			return resp, nil
		}
	}

	// Store offsets
	for topic, partitions := range req.Offsets {
		resp.Errors[topic] = make(map[int32]int16)
		for partition, data := range partitions {
			offsetMeta := &OffsetAndMetadata{
				Offset:     data.Offset,
				Metadata:   data.Metadata,
				CommitTime: time.Now(),
				ExpireTime: time.Now().Add(time.Duration(gc.config.OffsetRetentionMs) * time.Millisecond),
			}

			err := gc.offsetStorage.StoreOffset(req.GroupID, topic, partition, offsetMeta)
			if err != nil {
				resp.Errors[topic][partition] = ErrorCodeUnknown
			} else {
				resp.Errors[topic][partition] = ErrorCodeNone
			}
		}
	}

	return resp, nil
}

// HandleOffsetFetch handles an offset fetch request
func (gc *GroupCoordinator) HandleOffsetFetch(req *OffsetFetchRequest) (*OffsetFetchResponse, error) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	resp := &OffsetFetchResponse{
		Offsets: make(map[string]map[int32]OffsetFetchData),
	}

	// If no specific topics requested, fetch all
	if len(req.Topics) == 0 {
		groupOffsets, err := gc.offsetStorage.FetchOffsets(req.GroupID)
		if err != nil {
			return resp, err
		}

		for topic, partitions := range groupOffsets.Offsets {
			resp.Offsets[topic] = make(map[int32]OffsetFetchData)
			for partition, offsetMeta := range partitions {
				resp.Offsets[topic][partition] = OffsetFetchData{
					Offset:    offsetMeta.Offset,
					Metadata:  offsetMeta.Metadata,
					ErrorCode: ErrorCodeNone,
				}
			}
		}
	} else {
		// Fetch specific topics/partitions
		for topic, partitions := range req.Topics {
			resp.Offsets[topic] = make(map[int32]OffsetFetchData)
			for _, partition := range partitions {
				offsetMeta, err := gc.offsetStorage.FetchOffset(req.GroupID, topic, partition)
				if err != nil {
					resp.Offsets[topic][partition] = OffsetFetchData{
						ErrorCode: ErrorCodeUnknown,
					}
				} else if offsetMeta == nil {
					resp.Offsets[topic][partition] = OffsetFetchData{
						Offset:    -1, // No committed offset
						ErrorCode: ErrorCodeNone,
					}
				} else {
					resp.Offsets[topic][partition] = OffsetFetchData{
						Offset:    offsetMeta.Offset,
						Metadata:  offsetMeta.Metadata,
						ErrorCode: ErrorCodeNone,
					}
				}
			}
		}
	}

	return resp, nil
}

// Helper methods

func (gc *GroupCoordinator) getOrCreateGroup(groupID, protocolType string) *GroupMetadata {
	group, exists := gc.groups[groupID]
	if !exists {
		group = &GroupMetadata{
			GroupID:            groupID,
			State:              GroupStateEmpty,
			ProtocolType:       protocolType,
			Members:            make(map[string]*MemberMetadata),
			StateTimestamp:     time.Now(),
			SessionTimeoutMs:   gc.config.DefaultSessionTimeoutMs,
			RebalanceTimeoutMs: gc.config.DefaultRebalanceTimeoutMs,
		}
		gc.groups[groupID] = group

		gc.logger.Info("Created new consumer group", logging.Fields{
			"group_id": groupID,
		})
	}
	return group
}

func (gc *GroupCoordinator) getGroup(groupID string) (*GroupMetadata, *GroupError) {
	group, exists := gc.groups[groupID]
	if !exists {
		return nil, &GroupError{Code: ErrorCodeGroupIDNotFound}
	}
	return group, nil
}

func (gc *GroupCoordinator) generateMemberID(clientID string) string {
	return fmt.Sprintf("%s-%d", clientID, time.Now().UnixNano())
}

func (gc *GroupCoordinator) validateJoinRequest(req *JoinGroupRequest) *GroupError {
	if req.SessionTimeoutMs < gc.config.MinSessionTimeoutMs ||
		req.SessionTimeoutMs > gc.config.MaxSessionTimeoutMs {
		return &GroupError{Code: ErrorCodeInvalidSessionTimeout}
	}
	return nil
}

// heartbeatChecker periodically checks for expired members
func (gc *GroupCoordinator) heartbeatChecker() {
	defer gc.wg.Done()

	ticker := time.NewTicker(time.Duration(gc.config.HeartbeatCheckIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-gc.ctx.Done():
			return
		case <-ticker.C:
			gc.checkExpiredMembers()
		}
	}
}

func (gc *GroupCoordinator) checkExpiredMembers() {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	now := time.Now()

	for groupID, group := range gc.groups {
		expiredMembers := make([]string, 0)

		for memberID, member := range group.Members {
			timeout := time.Duration(member.SessionTimeoutMs) * time.Millisecond
			if now.Sub(member.LastHeartbeat) > timeout {
				expiredMembers = append(expiredMembers, memberID)
			}
		}

		// Remove expired members
		for _, memberID := range expiredMembers {
			delete(group.Members, memberID)
			gc.logger.Warn("Removed expired member", logging.Fields{
				"group_id":  groupID,
				"member_id": memberID,
			})
		}

		// Update group state if members expired
		if len(expiredMembers) > 0 {
			if len(group.Members) == 0 {
				group.State = GroupStateEmpty
			} else if group.State == GroupStateStable {
				group.State = GroupStatePreparingRebalance
				group.GenerationID++
				group.StateTimestamp = now
			}
		}
	}
}

// GroupError represents a group-related error
type GroupError struct {
	Code int16
}

func (e *GroupError) Error() string {
	return fmt.Sprintf("group error: code=%d", e.Code)
}
