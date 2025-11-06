package storage

import (
	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
)

const (
	maxLevel    = 32
	probability = 0.25
)

// memTableImpl implements MemTable using a skip list
type memTableImpl struct {
	head   *skipListNode
	level  int
	size   int64
	mu     sync.RWMutex
	rand   *rand.Rand
}

type skipListNode struct {
	key     []byte
	value   []byte
	forward []*skipListNode
}

// NewMemTable creates a new in-memory table
func NewMemTable() MemTable {
	return &memTableImpl{
		head:  &skipListNode{forward: make([]*skipListNode, maxLevel)},
		level: 1,
		rand:  rand.New(rand.NewSource(42)),
	}
}

func (m *memTableImpl) Put(key []byte, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the position to insert
	update := make([]*skipListNode, maxLevel)
	current := m.head

	for i := m.level - 1; i >= 0; i-- {
		for current.forward[i] != nil && bytes.Compare(current.forward[i].key, key) < 0 {
			current = current.forward[i]
		}
		update[i] = current
	}

	current = current.forward[0]

	// Update existing key
	if current != nil && bytes.Equal(current.key, key) {
		oldSize := int64(len(current.value))
		current.value = make([]byte, len(value))
		copy(current.value, value)
		atomic.AddInt64(&m.size, int64(len(value))-oldSize)
		return nil
	}

	// Insert new node
	level := m.randomLevel()
	if level > m.level {
		for i := m.level; i < level; i++ {
			update[i] = m.head
		}
		m.level = level
	}

	node := &skipListNode{
		key:     make([]byte, len(key)),
		value:   make([]byte, len(value)),
		forward: make([]*skipListNode, level),
	}
	copy(node.key, key)
	copy(node.value, value)

	for i := 0; i < level; i++ {
		node.forward[i] = update[i].forward[i]
		update[i].forward[i] = node
	}

	atomic.AddInt64(&m.size, int64(len(key)+len(value)))

	return nil
}

func (m *memTableImpl) Get(key []byte) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	current := m.head

	for i := m.level - 1; i >= 0; i-- {
		for current.forward[i] != nil && bytes.Compare(current.forward[i].key, key) < 0 {
			current = current.forward[i]
		}
	}

	current = current.forward[0]

	if current != nil && bytes.Equal(current.key, key) {
		value := make([]byte, len(current.value))
		copy(value, current.value)
		return value, true, nil
	}

	return nil, false, nil
}

func (m *memTableImpl) Delete(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	update := make([]*skipListNode, maxLevel)
	current := m.head

	for i := m.level - 1; i >= 0; i-- {
		for current.forward[i] != nil && bytes.Compare(current.forward[i].key, key) < 0 {
			current = current.forward[i]
		}
		update[i] = current
	}

	current = current.forward[0]

	if current != nil && bytes.Equal(current.key, key) {
		for i := 0; i < m.level; i++ {
			if update[i].forward[i] != current {
				break
			}
			update[i].forward[i] = current.forward[i]
		}

		atomic.AddInt64(&m.size, -int64(len(current.key)+len(current.value)))

		// Adjust level
		for m.level > 1 && m.head.forward[m.level-1] == nil {
			m.level--
		}
	}

	return nil
}

func (m *memTableImpl) Size() int64 {
	return atomic.LoadInt64(&m.size)
}

func (m *memTableImpl) Iterator() Iterator {
	m.mu.RLock()
	// Note: In production, we'd implement a proper snapshot iterator
	// that doesn't hold the read lock for the entire iteration
	return &memTableIterator{
		memTable: m,
		current:  m.head, // Start at head, Next() will advance to first element
		mu:       &m.mu,
	}
}

func (m *memTableImpl) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.head = &skipListNode{forward: make([]*skipListNode, maxLevel)}
	m.level = 1
	atomic.StoreInt64(&m.size, 0)
}

func (m *memTableImpl) randomLevel() int {
	level := 1
	for level < maxLevel && m.rand.Float64() < probability {
		level++
	}
	return level
}

// memTableIterator implements Iterator for MemTable
type memTableIterator struct {
	memTable *memTableImpl
	current  *skipListNode
	mu       *sync.RWMutex
	err      error
}

func (it *memTableIterator) Next() bool {
	if it.current != nil {
		it.current = it.current.forward[0]
	}
	return it.current != nil
}

func (it *memTableIterator) Key() []byte {
	if it.current == nil {
		return nil
	}
	return it.current.key
}

func (it *memTableIterator) Value() []byte {
	if it.current == nil {
		return nil
	}
	return it.current.value
}

func (it *memTableIterator) Err() error {
	return it.err
}

func (it *memTableIterator) Close() error {
	it.mu.RUnlock()
	return nil
}
