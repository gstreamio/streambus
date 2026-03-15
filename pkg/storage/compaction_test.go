package storage

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"
)

// --- Test helpers ---

// makeMemTableWithEntries creates a MemTable populated with the given key-value pairs.
func makeMemTableWithEntries(entries map[string]string) MemTable {
	mt := NewMemTable()
	for k, v := range entries {
		_ = mt.Put([]byte(k), []byte(v))
	}
	return mt
}

// makeMemTableWithSize creates a MemTable with enough data to reach approximately
// the given byte size.
func makeMemTableWithSize(targetSize int64) MemTable {
	mt := NewMemTable()
	i := 0
	for mt.Size() < targetSize {
		key := fmt.Sprintf("key-%06d", i)
		value := fmt.Sprintf("value-%06d", i)
		_ = mt.Put([]byte(key), []byte(value))
		i++
	}
	return mt
}

// timestampedTableSeq is a package-level counter to ensure unique keys across tables.
var timestampedTableSeq int

// makeTimestampedMemTable creates a MemTable with entries that have serialized
// message data containing the given timestamp. Uses the same format as
// logImpl.serializeMessage. Each call produces globally unique keys.
func makeTimestampedMemTable(ts time.Time, numEntries int) MemTable {
	mt := NewMemTable()
	seq := timestampedTableSeq
	timestampedTableSeq++
	for i := 0; i < numEntries; i++ {
		key := fmt.Sprintf("ts-key-%d-%06d", seq, i)
		value := serializeTestMessage(ts, []byte(key), []byte(fmt.Sprintf("ts-value-%d-%06d", seq, i)))
		_ = mt.Put([]byte(key), value)
	}
	return mt
}

// serializeTestMessage serializes a message with timestamp in the same format
// as logImpl.serializeMessage: [Timestamp:8][KeyLen:4][Key:n][ValueLen:4][Value:n]
func serializeTestMessage(ts time.Time, msgKey, msgValue []byte) []byte {
	size := 8 + 4 + len(msgKey) + 4 + len(msgValue)
	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], uint64(ts.UnixNano()))
	offset += 8
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msgKey)))
	offset += 4
	copy(buf[offset:], msgKey)
	offset += len(msgKey)
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msgValue)))
	offset += 4
	copy(buf[offset:], msgValue)

	return buf
}

// collectEntries reads all entries from a MemTable into a map.
func collectEntries(t *testing.T, mt MemTable) map[string]string {
	t.Helper()
	entries := make(map[string]string)
	iter := mt.Iterator()
	defer iter.Close()
	for iter.Next() {
		entries[string(iter.Key())] = string(iter.Value())
	}
	if iter.Err() != nil {
		t.Fatalf("iterator error: %v", iter.Err())
	}
	return entries
}

// --- newCompactor tests ---

func TestNewCompactor(t *testing.T) {
	tests := []struct {
		name     string
		strategy CompactionStrategy
		wantType string
	}{
		{
			name:     "leveled strategy",
			strategy: CompactionLeveled,
			wantType: "*storage.leveledCompactor",
		},
		{
			name:     "size-tiered strategy",
			strategy: CompactionSizeTiered,
			wantType: "*storage.sizeTieredCompactor",
		},
		{
			name:     "time-window strategy",
			strategy: CompactionTimeWindow,
			wantType: "*storage.timeWindowCompactor",
		},
		{
			name:     "unknown strategy defaults to leveled",
			strategy: CompactionStrategy(99),
			wantType: "*storage.leveledCompactor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCompactor(tt.strategy)
			got := fmt.Sprintf("%T", c)
			if got != tt.wantType {
				t.Errorf("newCompactor(%d) = %s, want %s", tt.strategy, got, tt.wantType)
			}
		})
	}
}

// --- Leveled Compaction tests ---

func TestLeveledCompaction(t *testing.T) {
	tests := []struct {
		name            string
		tables          []MemTable
		config          CompactionConfig
		wantMinOutput   int
		wantMaxOutput   int
		wantInputTables int
	}{
		{
			name:            "empty input",
			tables:          nil,
			config:          CompactionConfig{Strategy: CompactionLeveled, SizeRatio: 10},
			wantMinOutput:   0,
			wantMaxOutput:   0,
			wantInputTables: 0,
		},
		{
			name:            "single table unchanged",
			tables:          []MemTable{makeMemTableWithEntries(map[string]string{"a": "1"})},
			config:          CompactionConfig{Strategy: CompactionLeveled, SizeRatio: 10},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 1,
		},
		{
			name: "two similar-sized tables merge",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1", "b": "2"}),
				makeMemTableWithEntries(map[string]string{"c": "3", "d": "4"}),
			},
			config:          CompactionConfig{Strategy: CompactionLeveled, SizeRatio: 10},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 2,
		},
		{
			name: "multiple tables same level merge",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1"}),
				makeMemTableWithEntries(map[string]string{"b": "2"}),
				makeMemTableWithEntries(map[string]string{"c": "3"}),
			},
			config:          CompactionConfig{Strategy: CompactionLeveled, SizeRatio: 10},
			wantMinOutput:   1,
			wantMaxOutput:   2,
			wantInputTables: 3,
		},
		{
			name: "different-sized tables may stay separate",
			tables: []MemTable{
				makeMemTableWithSize(100),
				makeMemTableWithSize(100000),
			},
			config:          CompactionConfig{Strategy: CompactionLeveled, SizeRatio: 10},
			wantMinOutput:   1,
			wantMaxOutput:   2,
			wantInputTables: 2,
		},
		{
			name: "default size ratio when zero",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1"}),
				makeMemTableWithEntries(map[string]string{"b": "2"}),
			},
			config:          CompactionConfig{Strategy: CompactionLeveled, SizeRatio: 0},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &leveledCompactor{}
			output, result, err := c.compact(tt.tables, tt.config)
			if err != nil {
				t.Fatalf("compact() error = %v", err)
			}

			if result.InputTables != tt.wantInputTables {
				t.Errorf("InputTables = %d, want %d", result.InputTables, tt.wantInputTables)
			}

			if len(output) < tt.wantMinOutput || len(output) > tt.wantMaxOutput {
				t.Errorf("output tables = %d, want [%d, %d]", len(output), tt.wantMinOutput, tt.wantMaxOutput)
			}
		})
	}
}

func TestLeveledCompaction_DataPreservation(t *testing.T) {
	tables := []MemTable{
		makeMemTableWithEntries(map[string]string{"a": "1", "b": "2"}),
		makeMemTableWithEntries(map[string]string{"c": "3", "d": "4"}),
	}

	c := &leveledCompactor{}
	output, _, err := c.compact(tables, CompactionConfig{SizeRatio: 10})
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}

	// Collect all entries from output tables
	allEntries := make(map[string]string)
	for _, mt := range output {
		entries := collectEntries(t, mt)
		for k, v := range entries {
			allEntries[k] = v
		}
	}

	// Verify all original entries are preserved
	expected := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}
	for k, v := range expected {
		if got, ok := allEntries[k]; !ok {
			t.Errorf("missing key %q after compaction", k)
		} else if got != v {
			t.Errorf("key %q = %q, want %q", k, got, v)
		}
	}
}

func TestLeveledCompaction_DuplicateKeyLastWins(t *testing.T) {
	// Second table has the same key with a newer value
	tables := []MemTable{
		makeMemTableWithEntries(map[string]string{"a": "old"}),
		makeMemTableWithEntries(map[string]string{"a": "new"}),
	}

	c := &leveledCompactor{}
	output, _, err := c.compact(tables, CompactionConfig{SizeRatio: 10})
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}

	// Find key "a" in output
	for _, mt := range output {
		val, found, err := mt.Get([]byte("a"))
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if found {
			if string(val) != "new" {
				t.Errorf("duplicate key 'a' = %q, want 'new'", string(val))
			}
			return
		}
	}
	t.Error("key 'a' not found in compacted output")
}

func TestLeveledCompaction_ComputeLevel(t *testing.T) {
	lc := &leveledCompactor{}

	tests := []struct {
		name          string
		size          int64
		baseThreshold int64
		sizeRatio     int64
		wantLevel     int
	}{
		{"zero size", 0, 1024, 10, 0},
		{"below threshold", 500, 1024, 10, 0},
		{"at threshold", 1024, 1024, 10, 0},
		{"above threshold", 5000, 1024, 10, 1},
		{"10x threshold", 10240, 1024, 10, 1},
		{"above 10x threshold", 10241, 1024, 10, 2},
		{"large value", 200000, 1024, 10, 3},
		{"zero base threshold", 1024, 0, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lc.computeLevel(tt.size, tt.baseThreshold, tt.sizeRatio)
			if got != tt.wantLevel {
				t.Errorf("computeLevel(%d, %d, %d) = %d, want %d",
					tt.size, tt.baseThreshold, tt.sizeRatio, got, tt.wantLevel)
			}
		})
	}
}

// --- Size-Tiered Compaction tests ---

func TestSizeTieredCompaction(t *testing.T) {
	tests := []struct {
		name            string
		tables          []MemTable
		config          CompactionConfig
		wantMinOutput   int
		wantMaxOutput   int
		wantInputTables int
	}{
		{
			name:            "empty input",
			tables:          nil,
			config:          CompactionConfig{Strategy: CompactionSizeTiered},
			wantMinOutput:   0,
			wantMaxOutput:   0,
			wantInputTables: 0,
		},
		{
			name:            "single table unchanged",
			tables:          []MemTable{makeMemTableWithEntries(map[string]string{"a": "1"})},
			config:          CompactionConfig{Strategy: CompactionSizeTiered},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 1,
		},
		{
			name: "fewer than 4 similar tables not merged",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1"}),
				makeMemTableWithEntries(map[string]string{"b": "2"}),
				makeMemTableWithEntries(map[string]string{"c": "3"}),
			},
			config:          CompactionConfig{Strategy: CompactionSizeTiered},
			wantMinOutput:   3,
			wantMaxOutput:   3,
			wantInputTables: 3,
		},
		{
			name: "4 similar tables merge into 1",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1"}),
				makeMemTableWithEntries(map[string]string{"b": "2"}),
				makeMemTableWithEntries(map[string]string{"c": "3"}),
				makeMemTableWithEntries(map[string]string{"d": "4"}),
			},
			config:          CompactionConfig{Strategy: CompactionSizeTiered},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 4,
		},
		{
			name: "5 similar tables merge into 1",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1"}),
				makeMemTableWithEntries(map[string]string{"b": "2"}),
				makeMemTableWithEntries(map[string]string{"c": "3"}),
				makeMemTableWithEntries(map[string]string{"d": "4"}),
				makeMemTableWithEntries(map[string]string{"e": "5"}),
			},
			config:          CompactionConfig{Strategy: CompactionSizeTiered},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &sizeTieredCompactor{}
			output, result, err := c.compact(tt.tables, tt.config)
			if err != nil {
				t.Fatalf("compact() error = %v", err)
			}

			if result.InputTables != tt.wantInputTables {
				t.Errorf("InputTables = %d, want %d", result.InputTables, tt.wantInputTables)
			}

			if len(output) < tt.wantMinOutput || len(output) > tt.wantMaxOutput {
				t.Errorf("output tables = %d, want [%d, %d]", len(output), tt.wantMinOutput, tt.wantMaxOutput)
			}
		})
	}
}

func TestSizeTieredCompaction_DataPreservation(t *testing.T) {
	tables := []MemTable{
		makeMemTableWithEntries(map[string]string{"a": "1"}),
		makeMemTableWithEntries(map[string]string{"b": "2"}),
		makeMemTableWithEntries(map[string]string{"c": "3"}),
		makeMemTableWithEntries(map[string]string{"d": "4"}),
	}

	c := &sizeTieredCompactor{}
	output, _, err := c.compact(tables, CompactionConfig{})
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}

	allEntries := make(map[string]string)
	for _, mt := range output {
		entries := collectEntries(t, mt)
		for k, v := range entries {
			allEntries[k] = v
		}
	}

	expected := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}
	for k, v := range expected {
		if got, ok := allEntries[k]; !ok {
			t.Errorf("missing key %q after compaction", k)
		} else if got != v {
			t.Errorf("key %q = %q, want %q", k, got, v)
		}
	}
}

func TestSizeTieredCompaction_DifferentSizeBuckets(t *testing.T) {
	// Create 4 small tables and 1 large table
	tables := []MemTable{
		makeMemTableWithSize(100),
		makeMemTableWithSize(100),
		makeMemTableWithSize(100),
		makeMemTableWithSize(100),
		makeMemTableWithSize(10000),
	}

	c := &sizeTieredCompactor{}
	output, result, err := c.compact(tables, CompactionConfig{})
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}

	if result.InputTables != 5 {
		t.Errorf("InputTables = %d, want 5", result.InputTables)
	}

	// The 4 small tables should merge, the large one stays separate
	if len(output) != 2 {
		t.Errorf("output tables = %d, want 2 (1 merged small + 1 large)", len(output))
	}
}

func TestSizeTieredCompaction_GroupBySize(t *testing.T) {
	sc := &sizeTieredCompactor{}

	tests := []struct {
		name        string
		sizes       []int64
		wantBuckets int
	}{
		{
			name:        "all same size",
			sizes:       []int64{100, 100, 100, 100},
			wantBuckets: 1,
		},
		{
			name:        "two distinct sizes",
			sizes:       []int64{100, 100, 10000, 10000},
			wantBuckets: 2,
		},
		{
			name:        "single table",
			sizes:       []int64{100},
			wantBuckets: 1,
		},
		{
			name:        "gradually increasing within 2x",
			sizes:       []int64{100, 150, 180, 200},
			wantBuckets: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := make([]MemTable, len(tt.sizes))
			for i, size := range tt.sizes {
				tables[i] = makeMemTableWithSize(size)
			}

			buckets := sc.groupBySize(tables)
			if len(buckets) != tt.wantBuckets {
				t.Errorf("groupBySize() = %d buckets, want %d", len(buckets), tt.wantBuckets)
			}
		})
	}
}

// --- Time-Window Compaction tests ---

func TestTimeWindowCompaction(t *testing.T) {
	// Truncate to start of hour so now and now+10min are guaranteed in the same window
	now := time.Now().Truncate(time.Hour)

	tests := []struct {
		name            string
		tables          []MemTable
		config          CompactionConfig
		wantMinOutput   int
		wantMaxOutput   int
		wantInputTables int
	}{
		{
			name:            "empty input",
			tables:          nil,
			config:          CompactionConfig{Strategy: CompactionTimeWindow},
			wantMinOutput:   0,
			wantMaxOutput:   0,
			wantInputTables: 0,
		},
		{
			name: "single table unchanged",
			tables: []MemTable{
				makeTimestampedMemTable(now, 3),
			},
			config:          CompactionConfig{Strategy: CompactionTimeWindow},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 1,
		},
		{
			name: "same window tables merge",
			tables: []MemTable{
				makeTimestampedMemTable(now, 3),
				makeTimestampedMemTable(now.Add(10*time.Minute), 3),
			},
			config:          CompactionConfig{Strategy: CompactionTimeWindow},
			wantMinOutput:   1,
			wantMaxOutput:   1,
			wantInputTables: 2,
		},
		{
			name: "different window tables stay separate",
			tables: []MemTable{
				makeTimestampedMemTable(now, 3),
				makeTimestampedMemTable(now.Add(2*time.Hour), 3),
			},
			config:          CompactionConfig{Strategy: CompactionTimeWindow},
			wantMinOutput:   2,
			wantMaxOutput:   2,
			wantInputTables: 2,
		},
		{
			name: "three windows with one having 2 tables",
			tables: []MemTable{
				makeTimestampedMemTable(now, 2),
				makeTimestampedMemTable(now.Add(10*time.Minute), 2),
				makeTimestampedMemTable(now.Add(3*time.Hour), 2),
			},
			config:          CompactionConfig{Strategy: CompactionTimeWindow},
			wantMinOutput:   2,
			wantMaxOutput:   2,
			wantInputTables: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &timeWindowCompactor{}
			output, result, err := c.compact(tt.tables, tt.config)
			if err != nil {
				t.Fatalf("compact() error = %v", err)
			}

			if result.InputTables != tt.wantInputTables {
				t.Errorf("InputTables = %d, want %d", result.InputTables, tt.wantInputTables)
			}

			if len(output) < tt.wantMinOutput || len(output) > tt.wantMaxOutput {
				t.Errorf("output tables = %d, want [%d, %d]", len(output), tt.wantMinOutput, tt.wantMaxOutput)
			}
		})
	}
}

func TestTimeWindowCompaction_DataPreservation(t *testing.T) {
	now := time.Now()

	tables := []MemTable{
		makeTimestampedMemTable(now, 3),
		makeTimestampedMemTable(now.Add(10*time.Minute), 3),
	}

	// Count total entries before compaction
	totalBefore := 0
	for _, mt := range tables {
		totalBefore += countEntries(mt)
	}

	c := &timeWindowCompactor{}
	output, _, err := c.compact(tables, CompactionConfig{})
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}

	// Count total entries after compaction
	totalAfter := 0
	for _, mt := range output {
		totalAfter += countEntries(mt)
	}

	if totalAfter != totalBefore {
		t.Errorf("entry count changed: before=%d, after=%d", totalBefore, totalAfter)
	}
}

func TestTimeWindowCompaction_NeverMergesAcrossWindows(t *testing.T) {
	now := time.Now().Truncate(time.Hour)

	// Create tables in 3 different hourly windows
	window1 := []MemTable{
		makeTimestampedMemTable(now, 2),
		makeTimestampedMemTable(now.Add(30*time.Minute), 2),
	}
	window2 := []MemTable{
		makeTimestampedMemTable(now.Add(2*time.Hour), 2),
	}
	window3 := []MemTable{
		makeTimestampedMemTable(now.Add(5*time.Hour), 2),
		makeTimestampedMemTable(now.Add(5*time.Hour+30*time.Minute), 2),
	}

	allTables := append(append(window1, window2...), window3...)

	c := &timeWindowCompactor{}
	output, result, err := c.compact(allTables, CompactionConfig{})
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}

	if result.InputTables != 5 {
		t.Errorf("InputTables = %d, want 5", result.InputTables)
	}

	// Window 1 has 2 tables -> merge to 1
	// Window 2 has 1 table -> stays 1
	// Window 3 has 2 tables -> merge to 1
	// Total output: 3
	if len(output) != 3 {
		t.Errorf("output tables = %d, want 3", len(output))
	}
}

func TestTimeWindowCompaction_GetWindowKey(t *testing.T) {
	tc := &timeWindowCompactor{}

	baseTime := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		ts       time.Time
		window   time.Duration
		wantSame bool // whether two times should map to the same window
		otherTs  time.Time
	}{
		{
			name:     "same hour same window",
			ts:       baseTime,
			window:   time.Hour,
			wantSame: true,
			otherTs:  baseTime.Add(30 * time.Minute),
		},
		{
			name:     "different hours different window",
			ts:       baseTime,
			window:   time.Hour,
			wantSame: false,
			otherTs:  baseTime.Add(2 * time.Hour),
		},
		{
			name:     "same day same window",
			ts:       baseTime,
			window:   24 * time.Hour,
			wantSame: true,
			otherTs:  baseTime.Add(12 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt1 := makeTimestampedMemTable(tt.ts, 1)
			mt2 := makeTimestampedMemTable(tt.otherTs, 1)

			key1 := tc.getWindowKey(mt1, tt.window)
			key2 := tc.getWindowKey(mt2, tt.window)

			if tt.wantSame && key1 != key2 {
				t.Errorf("expected same window key for %v and %v, got %d and %d",
					tt.ts, tt.otherTs, key1, key2)
			}
			if !tt.wantSame && key1 == key2 {
				t.Errorf("expected different window keys for %v and %v, both got %d",
					tt.ts, tt.otherTs, key1)
			}
		})
	}
}

func TestTimeWindowCompaction_EmptyMemTable(t *testing.T) {
	tc := &timeWindowCompactor{}

	emptyMT := NewMemTable()
	key := tc.getWindowKey(emptyMT, time.Hour)

	if key != 0 {
		t.Errorf("expected window key 0 for empty memtable, got %d", key)
	}
}

// --- Shared helper tests ---

func TestMergeMemTables(t *testing.T) {
	tests := []struct {
		name           string
		tables         []MemTable
		wantEntries    map[string]string
		wantEntryCount int
	}{
		{
			name:           "empty input",
			tables:         nil,
			wantEntries:    map[string]string{},
			wantEntryCount: 0,
		},
		{
			name: "single table",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1", "b": "2"}),
			},
			wantEntries:    map[string]string{"a": "1", "b": "2"},
			wantEntryCount: 2,
		},
		{
			name: "two disjoint tables",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "1"}),
				makeMemTableWithEntries(map[string]string{"b": "2"}),
			},
			wantEntries:    map[string]string{"a": "1", "b": "2"},
			wantEntryCount: 2,
		},
		{
			name: "overlapping keys last wins",
			tables: []MemTable{
				makeMemTableWithEntries(map[string]string{"a": "old", "b": "2"}),
				makeMemTableWithEntries(map[string]string{"a": "new", "c": "3"}),
			},
			wantEntries:    map[string]string{"a": "new", "b": "2", "c": "3"},
			wantEntryCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := mergeMemTables(tt.tables)
			entries := collectEntries(t, merged)

			if len(entries) != tt.wantEntryCount {
				t.Errorf("entry count = %d, want %d", len(entries), tt.wantEntryCount)
			}

			for k, v := range tt.wantEntries {
				if got, ok := entries[k]; !ok {
					t.Errorf("missing key %q", k)
				} else if got != v {
					t.Errorf("key %q = %q, want %q", k, got, v)
				}
			}
		})
	}
}

func TestCountEntries(t *testing.T) {
	tests := []struct {
		name  string
		table MemTable
		want  int
	}{
		{
			name:  "empty table",
			table: NewMemTable(),
			want:  0,
		},
		{
			name:  "table with entries",
			table: makeMemTableWithEntries(map[string]string{"a": "1", "b": "2", "c": "3"}),
			want:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countEntries(tt.table)
			if got != tt.want {
				t.Errorf("countEntries() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExtractTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantTs  time.Time
		isValid bool
	}{
		{
			name:    "empty data",
			data:    nil,
			wantTs:  time.Time{},
			isValid: false,
		},
		{
			name:    "too short",
			data:    []byte{1, 2, 3},
			wantTs:  time.Time{},
			isValid: false,
		},
		{
			name:    "valid timestamped message",
			data:    serializeTestMessage(time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC), []byte("k"), []byte("v")),
			wantTs:  time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTimestamp(tt.data)
			if tt.isValid {
				if !got.Equal(tt.wantTs) {
					t.Errorf("extractTimestamp() = %v, want %v", got, tt.wantTs)
				}
			} else {
				if !got.IsZero() {
					t.Errorf("extractTimestamp() = %v, want zero time", got)
				}
			}
		})
	}
}

// --- Integration tests through logImpl.Compact() ---

func TestLog_Compact_Leveled(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      100, // Small to trigger rotation
			NumImmutable: 10,
		},
		Compaction: CompactionConfig{
			Strategy:  CompactionLeveled,
			SizeRatio: 10,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Append enough messages to create immutable memtables
	for i := 0; i < 20; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{Key: []byte(fmt.Sprintf("key-%d", i)), Value: make([]byte, 50)},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Compact should succeed
	if err := log.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	// Data should still be readable
	hwm := log.HighWaterMark()
	if hwm != 20 {
		t.Errorf("HighWaterMark = %d, want 20", hwm)
	}
}

func TestLog_Compact_SizeTiered(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      100,
			NumImmutable: 10,
		},
		Compaction: CompactionConfig{
			Strategy: CompactionSizeTiered,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	for i := 0; i < 20; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{Key: []byte(fmt.Sprintf("key-%d", i)), Value: make([]byte, 50)},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	if err := log.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	hwm := log.HighWaterMark()
	if hwm != 20 {
		t.Errorf("HighWaterMark = %d, want 20", hwm)
	}
}

func TestLog_Compact_TimeWindow(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      100,
			NumImmutable: 10,
		},
		Compaction: CompactionConfig{
			Strategy: CompactionTimeWindow,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	now := time.Now()
	for i := 0; i < 20; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{
					Key:       []byte(fmt.Sprintf("key-%d", i)),
					Value:     make([]byte, 50),
					Timestamp: now.Add(time.Duration(i) * time.Minute),
				},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	if err := log.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	hwm := log.HighWaterMark()
	if hwm != 20 {
		t.Errorf("HighWaterMark = %d, want 20", hwm)
	}
}

func TestLog_Compact_NoImmutableTables(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024, // Large enough that no rotation happens
			NumImmutable: 2,
		},
		Compaction: CompactionConfig{
			Strategy: CompactionLeveled,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Append a small batch that won't trigger rotation
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key"), Value: []byte("value")},
		},
	}
	if _, err := log.Append(batch); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Compact with no immutable tables should be a no-op
	if err := log.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
}

func TestLog_Compact_ClosedLog(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	log.Close()

	err = log.Compact()
	if err != ErrLogClosed {
		t.Errorf("Expected ErrLogClosed, got %v", err)
	}
}

func TestLog_Compact_AllStrategies_ReadAfterCompact(t *testing.T) {
	strategies := []struct {
		name     string
		strategy CompactionStrategy
	}{
		{"leveled", CompactionLeveled},
		{"size-tiered", CompactionSizeTiered},
		{"time-window", CompactionTimeWindow},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			dir := t.TempDir()

			config := Config{
				DataDir: dir,
				WAL: WALConfig{
					SegmentSize:   1024 * 1024,
					FsyncPolicy:   FsyncAlways,
					FsyncInterval: time.Second,
				},
				MemTable: MemTableConfig{
					MaxSize:      100,
					NumImmutable: 10,
				},
				Compaction: CompactionConfig{
					Strategy:  s.strategy,
					SizeRatio: 10,
				},
			}

			log, err := NewLog(dir, config)
			if err != nil {
				t.Fatalf("Failed to create log: %v", err)
			}
			defer log.Close()

			// Write messages
			numMessages := 15
			now := time.Now()
			for i := 0; i < numMessages; i++ {
				batch := &MessageBatch{
					Messages: []Message{
						{
							Key:       []byte(fmt.Sprintf("key-%d", i)),
							Value:     make([]byte, 50),
							Timestamp: now.Add(time.Duration(i) * time.Minute),
						},
					},
				}
				if _, err := log.Append(batch); err != nil {
					t.Fatalf("Append %d failed: %v", i, err)
				}
			}

			// Compact
			if err := log.Compact(); err != nil {
				t.Fatalf("Compact failed: %v", err)
			}

			// Verify all messages are still readable via WAL
			messages, err := log.Read(0, 1024*1024)
			if err != nil {
				t.Fatalf("Read after compact failed: %v", err)
			}

			if len(messages) != numMessages {
				t.Errorf("Expected %d messages after compact, got %d", numMessages, len(messages))
			}
		})
	}
}
