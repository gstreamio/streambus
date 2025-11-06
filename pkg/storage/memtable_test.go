package storage

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMemTable_PutAndGet(t *testing.T) {
	mt := NewMemTable()

	tests := []struct {
		key   []byte
		value []byte
	}{
		{[]byte("key1"), []byte("value1")},
		{[]byte("key2"), []byte("value2")},
		{[]byte("key3"), []byte("value3")},
	}

	// Put values
	for _, tt := range tests {
		err := mt.Put(tt.key, tt.value)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Get values
	for _, tt := range tests {
		value, found, err := mt.Get(tt.key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !found {
			t.Fatalf("Key not found: %s", tt.key)
		}
		if !bytes.Equal(value, tt.value) {
			t.Fatalf("Value mismatch: got %s, want %s", value, tt.value)
		}
	}
}

func TestMemTable_Update(t *testing.T){
	mt := NewMemTable()

	key := []byte("key1")
	value1 := []byte("value1")
	value2 := []byte("value2-updated")

	// Initial put
	if err := mt.Put(key, value1); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Update
	if err := mt.Put(key, value2); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify updated value
	value, found, err := mt.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("Key not found")
	}
	if !bytes.Equal(value, value2) {
		t.Fatalf("Value mismatch: got %s, want %s", value, value2)
	}
}

func TestMemTable_Delete(t *testing.T) {
	mt := NewMemTable()

	key := []byte("key1")
	value := []byte("value1")

	// Put
	if err := mt.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify exists
	_, found, _ := mt.Get(key)
	if !found {
		t.Fatal("Key should exist")
	}

	// Delete
	if err := mt.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, found, _ = mt.Get(key)
	if found {
		t.Fatal("Key should not exist after delete")
	}
}

func TestMemTable_Size(t *testing.T) {
	mt := NewMemTable()

	if mt.Size() != 0 {
		t.Fatalf("Expected size 0, got %d", mt.Size())
	}

	key := []byte("key1")
	value := []byte("value1")

	mt.Put(key, value)

	expectedSize := int64(len(key) + len(value))
	if mt.Size() != expectedSize {
		t.Fatalf("Expected size %d, got %d", expectedSize, mt.Size())
	}
}

func TestMemTable_Iterator(t *testing.T) {
	mt := NewMemTable()

	// Insert data in non-sorted order
	data := map[string]string{
		"c": "value3",
		"a": "value1",
		"b": "value2",
	}

	for k, v := range data {
		mt.Put([]byte(k), []byte(v))
	}

	// Iterate and verify order
	it := mt.Iterator()
	defer it.Close()

	expectedOrder := []string{"a", "b", "c"}
	i := 0

	for it.Next() {
		if i >= len(expectedOrder) {
			t.Fatal("Too many elements in iterator")
		}

		key := string(it.Key())
		if key != expectedOrder[i] {
			t.Fatalf("Expected key %s, got %s", expectedOrder[i], key)
		}

		i++
	}

	if i != len(expectedOrder) {
		t.Fatalf("Expected %d elements, got %d", len(expectedOrder), i)
	}

	if err := it.Err(); err != nil {
		t.Fatalf("Iterator error: %v", err)
	}
}

func TestMemTable_Clear(t *testing.T) {
	mt := NewMemTable()

	// Add data
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("key%d", i))
		value := []byte(fmt.Sprintf("value%d", i))
		mt.Put(key, value)
	}

	if mt.Size() == 0 {
		t.Fatal("MemTable should not be empty")
	}

	// Clear
	mt.Clear()

	if mt.Size() != 0 {
		t.Fatalf("Expected size 0 after clear, got %d", mt.Size())
	}

	// Verify data is gone
	_, found, _ := mt.Get([]byte("key0"))
	if found {
		t.Fatal("MemTable should be empty after clear")
	}
}

func BenchmarkMemTable_Put(b *testing.B) {
	mt := NewMemTable()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key%d", i))
		value := []byte(fmt.Sprintf("value%d", i))
		mt.Put(key, value)
	}
}

func BenchmarkMemTable_Get(b *testing.B) {
	mt := NewMemTable()

	// Prepare data
	for i := 0; i < 10000; i++ {
		key := []byte(fmt.Sprintf("key%d", i))
		value := []byte(fmt.Sprintf("value%d", i))
		mt.Put(key, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key%d", i%10000))
		mt.Get(key)
	}
}

func BenchmarkMemTable_Iterator(b *testing.B) {
	mt := NewMemTable()

	// Prepare data
	for i := 0; i < 1000; i++ {
		key := []byte(fmt.Sprintf("key%04d", i))
		value := []byte(fmt.Sprintf("value%d", i))
		mt.Put(key, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		it := mt.Iterator()
		count := 0
		for it.Next() {
			_ = it.Key()
			_ = it.Value()
			count++
		}
		it.Close()
	}
}
