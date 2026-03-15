package protocol

import (
	"sync"
)

// BufferPool is a pool of reusable byte buffers to reduce allocations
// This implements zero-copy optimization by reusing buffers across requests
type BufferPool struct {
	small  sync.Pool // For buffers up to 4KB
	medium sync.Pool // For buffers up to 64KB
	large  sync.Pool // For buffers up to 1MB
}

const (
	smallBufferSize  = 4 * 1024    // 4KB
	mediumBufferSize = 64 * 1024   // 64KB
	largeBufferSize  = 1024 * 1024 // 1MB
)

// NewBufferPool creates a new buffer pool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		small: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, smallBufferSize)
				return &buf
			},
		},
		medium: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, mediumBufferSize)
				return &buf
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, largeBufferSize)
				return &buf
			},
		},
	}
}

// Get returns a buffer of at least the requested size
// The returned buffer may be larger than requested
func (p *BufferPool) Get(size int) []byte {
	if size <= smallBufferSize {
		bufPtr := p.small.Get().(*[]byte)
		return (*bufPtr)[:size]
	} else if size <= mediumBufferSize {
		bufPtr := p.medium.Get().(*[]byte)
		return (*bufPtr)[:size]
	} else if size <= largeBufferSize {
		bufPtr := p.large.Get().(*[]byte)
		return (*bufPtr)[:size]
	}

	// For very large buffers, allocate directly (not pooled)
	return make([]byte, size)
}

// Put returns a buffer to the pool for reuse
// Only buffers obtained from Get() should be Put() back
func (p *BufferPool) Put(buf []byte) {
	// Reslice to full capacity
	buf = buf[:cap(buf)]

	capacity := cap(buf)
	if capacity == smallBufferSize {
		p.small.Put(&buf)
	} else if capacity == mediumBufferSize {
		p.medium.Put(&buf)
	} else if capacity == largeBufferSize {
		p.large.Put(&buf)
	}
	// Don't pool oversized buffers - let GC handle them
}

// GetExact returns a buffer of exactly the requested size
// Use this when you need precise sizing
func (p *BufferPool) GetExact(size int) []byte {
	buf := p.Get(size)
	if cap(buf) > size && cap(buf) > smallBufferSize {
		// If the pooled buffer is too large, allocate exact size
		return make([]byte, size)
	}
	return buf
}

// ZeroCopySlice creates a slice view into a buffer without copying
// The returned slice shares memory with the original buffer
// IMPORTANT: The original buffer must not be modified while this slice is in use
func ZeroCopySlice(buf []byte, offset, length int) []byte {
	if offset+length > len(buf) {
		return nil
	}
	return buf[offset : offset+length]
}

// SharedBufferWrapper wraps a buffer that should not be modified
// This is used for zero-copy reads where the buffer is shared
type SharedBufferWrapper struct {
	buf []byte
	// refCount could be added here for more sophisticated lifetime management
}

// NewSharedBuffer creates a new shared buffer wrapper
func NewSharedBuffer(buf []byte) *SharedBufferWrapper {
	return &SharedBufferWrapper{
		buf: buf,
	}
}

// Slice returns a zero-copy slice of the buffer
func (s *SharedBufferWrapper) Slice(offset, length int) []byte {
	return ZeroCopySlice(s.buf, offset, length)
}

// Bytes returns the full buffer (read-only view)
func (s *SharedBufferWrapper) Bytes() []byte {
	return s.buf
}

// Len returns the buffer length
func (s *SharedBufferWrapper) Len() int {
	return len(s.buf)
}

// MessageBuffer is a reusable buffer for encoding/decoding messages
// with zero-copy slice views
type MessageBuffer struct {
	buf    []byte
	offset int
	pool   *BufferPool
}

// NewMessageBuffer creates a new message buffer
func NewMessageBuffer(pool *BufferPool, initialSize int) *MessageBuffer {
	return &MessageBuffer{
		buf:    pool.Get(initialSize),
		offset: 0,
		pool:   pool,
	}
}

// Write appends data to the buffer (may require copying)
func (mb *MessageBuffer) Write(data []byte) {
	needed := mb.offset + len(data)
	if needed > cap(mb.buf) {
		// Need to grow - allocate new buffer
		newSize := needed * 2 // 2x growth
		newBuf := mb.pool.Get(newSize)
		// Extend slice to full capacity for writing
		newBuf = newBuf[:cap(newBuf)]
		copy(newBuf, mb.buf[:mb.offset])
		mb.pool.Put(mb.buf)
		mb.buf = newBuf
	}

	// Ensure buffer is large enough for write
	if needed > len(mb.buf) {
		mb.buf = mb.buf[:cap(mb.buf)]
	}

	copy(mb.buf[mb.offset:], data)
	mb.offset += len(data)
}

// WriteZeroCopy appends data reference without copying (if possible)
// Returns true if zero-copy was used, false if data was copied
func (mb *MessageBuffer) WriteZeroCopy(data []byte) bool {
	// Zero-copy is only possible if data directly follows current buffer
	// This is primarily useful for sequential reads from network/disk
	// For now, we use regular write - true zero-copy requires
	// vectored I/O or similar low-level support
	mb.Write(data)
	return false // Indicate copy was made
}

// Bytes returns the current buffer contents
func (mb *MessageBuffer) Bytes() []byte {
	return mb.buf[:mb.offset]
}

// Slice returns a zero-copy slice view
func (mb *MessageBuffer) Slice(offset, length int) []byte {
	return ZeroCopySlice(mb.buf[:mb.offset], offset, length)
}

// Reset resets the buffer for reuse
func (mb *MessageBuffer) Reset() {
	mb.offset = 0
	// Don't deallocate - keep buffer for reuse
}

// Release returns the buffer to the pool
func (mb *MessageBuffer) Release() {
	if mb.pool != nil && mb.buf != nil {
		mb.pool.Put(mb.buf)
		mb.buf = nil
	}
}

// Global default pool
var defaultPool = NewBufferPool()

// GetBuffer gets a buffer from the default pool
func GetBuffer(size int) []byte {
	return defaultPool.Get(size)
}

// PutBuffer returns a buffer to the default pool
func PutBuffer(buf []byte) {
	defaultPool.Put(buf)
}

// WithBuffer executes a function with a pooled buffer and automatically returns it
func WithBuffer(size int, fn func([]byte) error) error {
	buf := GetBuffer(size)
	defer PutBuffer(buf)
	return fn(buf)
}
