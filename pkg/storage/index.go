package storage

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"sync"
)

// indexImpl implements the Index interface
type indexImpl struct {
	path    string
	entries []indexEntry
	mu      sync.RWMutex
	file    *os.File
	dirty   bool
}

// indexEntry represents an offset-to-position mapping
type indexEntry struct {
	offset   Offset
	position int64
}

// Index entry format on disk:
// [Offset: 8 bytes][Position: 8 bytes] = 16 bytes per entry
const indexEntrySize = 16

// NewIndex creates a new index
func NewIndex(path string) (Index, error) {
	idx := &indexImpl{
		path:    path,
		entries: make([]indexEntry, 0),
	}

	// Try to load existing index
	if _, err := os.Stat(path); err == nil {
		if err := idx.load(); err != nil {
			return nil, err
		}
	} else {
		// Create new index file
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}
		idx.file = file
	}

	return idx, nil
}

func (idx *indexImpl) Lookup(offset Offset) (position int64, err error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.entries) == 0 {
		return 0, ErrOffsetOutOfRange
	}

	// Binary search for the closest offset <= target
	i := sort.Search(len(idx.entries), func(i int) bool {
		return idx.entries[i].offset > offset
	})

	if i == 0 {
		// Offset is before first entry
		return 0, ErrOffsetOutOfRange
	}

	// Return the position of the previous entry
	// The caller will scan from this position
	return idx.entries[i-1].position, nil
}

func (idx *indexImpl) Add(offset Offset, position int64) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Add entry
	idx.entries = append(idx.entries, indexEntry{
		offset:   offset,
		position: position,
	})

	idx.dirty = true

	return nil
}

func (idx *indexImpl) Truncate(beforeOffset Offset) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Find entries to keep
	var kept []indexEntry
	for _, entry := range idx.entries {
		if entry.offset >= beforeOffset {
			kept = append(kept, entry)
		}
	}

	idx.entries = kept
	idx.dirty = true

	return nil
}

func (idx *indexImpl) Sync() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.dirty || idx.file == nil {
		return nil
	}

	// Truncate file
	if err := idx.file.Truncate(0); err != nil {
		return err
	}

	// Seek to beginning
	if _, err := idx.file.Seek(0, 0); err != nil {
		return err
	}

	// Write all entries
	buf := make([]byte, indexEntrySize)
	for _, entry := range idx.entries {
		binary.BigEndian.PutUint64(buf[0:8], uint64(entry.offset))
		binary.BigEndian.PutUint64(buf[8:16], uint64(entry.position))

		if _, err := idx.file.Write(buf); err != nil {
			return err
		}
	}

	// Fsync
	if err := idx.file.Sync(); err != nil {
		return err
	}

	idx.dirty = false
	return nil
}

func (idx *indexImpl) Close() error {
	// Sync if needed (Sync() has its own lock)
	if idx.dirty {
		if err := idx.Sync(); err != nil {
			return err
		}
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.file != nil {
		return idx.file.Close()
	}

	return nil
}

func (idx *indexImpl) load() error {
	file, err := os.OpenFile(idx.path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	idx.file = file

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return err
	}

	size := stat.Size()
	if size%indexEntrySize != 0 {
		return fmt.Errorf("corrupt index file: size %d not multiple of %d", size, indexEntrySize)
	}

	numEntries := int(size / indexEntrySize)
	idx.entries = make([]indexEntry, 0, numEntries)

	// Read all entries
	buf := make([]byte, indexEntrySize)
	for i := 0; i < numEntries; i++ {
		if _, err := file.Read(buf); err != nil {
			return err
		}

		offset := Offset(binary.BigEndian.Uint64(buf[0:8]))
		position := int64(binary.BigEndian.Uint64(buf[8:16]))

		idx.entries = append(idx.entries, indexEntry{
			offset:   offset,
			position: position,
		})
	}

	return nil
}

// Size returns the number of entries in the index
func (idx *indexImpl) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}
