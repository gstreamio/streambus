package transaction

import (
	"fmt"
	"sync"
)

// MemoryTransactionLog is an in-memory implementation of TransactionLog
type MemoryTransactionLog struct {
	mu      sync.RWMutex
	entries map[TransactionID]*TransactionLogEntry
}

// NewMemoryTransactionLog creates a new in-memory transaction log
func NewMemoryTransactionLog() *MemoryTransactionLog {
	return &MemoryTransactionLog{
		entries: make(map[TransactionID]*TransactionLogEntry),
	}
}

// Append adds a new entry to the log
func (l *MemoryTransactionLog) Append(entry *TransactionLogEntry) error {
	if entry == nil {
		return fmt.Errorf("entry cannot be nil")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Create a copy of the entry
	entryCopy := *entry
	if entry.Partitions != nil {
		entryCopy.Partitions = make([]PartitionMetadata, len(entry.Partitions))
		copy(entryCopy.Partitions, entry.Partitions)
	}

	l.entries[entry.TransactionID] = &entryCopy

	return nil
}

// Read retrieves an entry from the log
func (l *MemoryTransactionLog) Read(txnID TransactionID) (*TransactionLogEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entry, exists := l.entries[txnID]
	if !exists {
		return nil, fmt.Errorf("transaction not found: %s", txnID)
	}

	// Return a copy
	entryCopy := *entry
	if entry.Partitions != nil {
		entryCopy.Partitions = make([]PartitionMetadata, len(entry.Partitions))
		copy(entryCopy.Partitions, entry.Partitions)
	}

	return &entryCopy, nil
}

// ReadAll retrieves all entries from the log
func (l *MemoryTransactionLog) ReadAll() ([]*TransactionLogEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entries := make([]*TransactionLogEntry, 0, len(l.entries))
	for _, entry := range l.entries {
		entryCopy := *entry
		if entry.Partitions != nil {
			entryCopy.Partitions = make([]PartitionMetadata, len(entry.Partitions))
			copy(entryCopy.Partitions, entry.Partitions)
		}
		entries = append(entries, &entryCopy)
	}

	return entries, nil
}

// Delete removes an entry from the log
func (l *MemoryTransactionLog) Delete(txnID TransactionID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.entries, txnID)
	return nil
}

// Count returns the number of entries in the log
func (l *MemoryTransactionLog) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// Clear removes all entries from the log
func (l *MemoryTransactionLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make(map[TransactionID]*TransactionLogEntry)
}
