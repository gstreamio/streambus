package protocol

import (
	"testing"
)

func TestBufferPool_GetPut(t *testing.T) {
	pool := NewBufferPool()

	// Test small buffer
	small := pool.Get(1024)
	if len(small) != 1024 {
		t.Errorf("Expected length 1024, got %d", len(small))
	}
	if cap(small) < 1024 {
		t.Errorf("Expected capacity >= 1024, got %d", cap(small))
	}

	// Modify buffer
	for i := range small {
		small[i] = byte(i % 256)
	}

	// Return to pool
	pool.Put(small)

	// Get again - should get same buffer (or at least same capacity)
	small2 := pool.Get(1024)
	if cap(small2) != cap(small) {
		t.Logf("Note: Got different capacity buffer (expected for concurrent tests)")
	}

	// Test medium buffer
	medium := pool.Get(32 * 1024)
	if len(medium) != 32*1024 {
		t.Errorf("Expected length 32KB, got %d", len(medium))
	}
	pool.Put(medium)

	// Test large buffer
	large := pool.Get(512 * 1024)
	if len(large) != 512*1024 {
		t.Errorf("Expected length 512KB, got %d", len(large))
	}
	pool.Put(large)

	// Test oversized buffer (not pooled)
	huge := pool.Get(2 * 1024 * 1024)
	if len(huge) != 2*1024*1024 {
		t.Errorf("Expected length 2MB, got %d", len(huge))
	}
	// Put doesn't pool oversized buffers
	pool.Put(huge)
}

func TestBufferPool_GetExact(t *testing.T) {
	pool := NewBufferPool()

	buf := pool.GetExact(2048)
	if len(buf) != 2048 {
		t.Errorf("Expected exact length 2048, got %d", len(buf))
	}

	pool.Put(buf)
}

func TestZeroCopySlice(t *testing.T) {
	original := []byte("Hello, World!")

	// Valid slice
	slice := ZeroCopySlice(original, 0, 5)
	if string(slice) != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", string(slice))
	}

	// Verify it's zero-copy (same underlying array)
	slice[0] = 'h'
	if original[0] != 'h' {
		t.Error("Modification didn't affect original - not zero-copy")
	}

	// Invalid slice (out of bounds)
	invalid := ZeroCopySlice(original, 0, 100)
	if invalid != nil {
		t.Error("Expected nil for out-of-bounds slice")
	}
}

func TestSharedBufferWrapper(t *testing.T) {
	buf := []byte("Test buffer content")
	shared := NewSharedBuffer(buf)

	if shared.Len() != len(buf) {
		t.Errorf("Expected length %d, got %d", len(buf), shared.Len())
	}

	// Get full buffer
	full := shared.Bytes()
	if string(full) != string(buf) {
		t.Error("Full buffer doesn't match original")
	}

	// Get slice
	slice := shared.Slice(5, 6)
	if string(slice) != "buffer" {
		t.Errorf("Expected 'buffer', got '%s'", string(slice))
	}

	// Invalid slice
	invalid := shared.Slice(100, 5)
	if invalid != nil {
		t.Error("Expected nil for out-of-bounds slice")
	}
}

func TestMessageBuffer(t *testing.T) {
	pool := NewBufferPool()
	mb := NewMessageBuffer(pool, 1024)
	defer mb.Release()

	// Write data
	data1 := []byte("Hello")
	mb.Write(data1)

	if string(mb.Bytes()) != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", string(mb.Bytes()))
	}

	// Write more data
	data2 := []byte(", World!")
	mb.Write(data2)

	if string(mb.Bytes()) != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", string(mb.Bytes()))
	}

	// Get slice
	slice := mb.Slice(0, 5)
	if string(slice) != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", string(slice))
	}

	// Reset
	mb.Reset()
	if len(mb.Bytes()) != 0 {
		t.Error("Buffer not empty after reset")
	}

	// Write after reset
	mb.Write([]byte("New data"))
	if string(mb.Bytes()) != "New data" {
		t.Errorf("Expected 'New data', got '%s'", string(mb.Bytes()))
	}
}

func TestMessageBuffer_Growth(t *testing.T) {
	pool := NewBufferPool()
	mb := NewMessageBuffer(pool, 10) // Small initial size
	defer mb.Release()

	// Write data larger than initial capacity
	largeData := make([]byte, 1000)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	mb.Write(largeData)

	if len(mb.Bytes()) != 1000 {
		t.Errorf("Expected length 1000, got %d", len(mb.Bytes()))
	}

	// Verify data integrity
	for i := range largeData {
		if mb.Bytes()[i] != largeData[i] {
			t.Errorf("Data mismatch at index %d", i)
			break
		}
	}
}

func TestWithBuffer(t *testing.T) {
	var bufLen int

	err := WithBuffer(1024, func(buf []byte) error {
		bufLen = len(buf)
		return nil
	})

	if err != nil {
		t.Fatalf("WithBuffer failed: %v", err)
	}

	if bufLen != 1024 {
		t.Errorf("Expected buffer length 1024, got %d", bufLen)
	}
}

// Benchmarks

func BenchmarkBufferPool_Get(b *testing.B) {
	pool := NewBufferPool()

	b.Run("Small", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := pool.Get(1024)
			pool.Put(buf)
		}
	})

	b.Run("Medium", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := pool.Get(32 * 1024)
			pool.Put(buf)
		}
	})

	b.Run("Large", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := pool.Get(512 * 1024)
			pool.Put(buf)
		}
	})
}

func BenchmarkBufferPool_vs_Make(b *testing.B) {
	pool := NewBufferPool()
	size := 4096

	b.Run("Pool", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := pool.Get(size)
			// Simulate usage
			_ = buf[0]
			pool.Put(buf)
		}
	})

	b.Run("Make", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := make([]byte, size)
			// Simulate usage
			_ = buf[0]
		}
	})
}

func BenchmarkZeroCopySlice(b *testing.B) {
	data := make([]byte, 1024)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		slice := ZeroCopySlice(data, 100, 500)
		_ = slice[0]
	}
}

func BenchmarkMessageBuffer_Write(b *testing.B) {
	pool := NewBufferPool()
	data := []byte("Test message data")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mb := NewMessageBuffer(pool, 1024)
		mb.Write(data)
		mb.Release()
	}
}
