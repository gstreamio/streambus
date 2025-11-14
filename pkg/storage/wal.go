package storage

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// walImpl implements the WAL interface
type walImpl struct {
	config WALConfig
	dir    string

	mu             sync.RWMutex
	currentSegment *walSegment
	segments       []*walSegment
	nextOffset     Offset

	closed bool

	// For async fsync
	fsyncTicker *time.Ticker
	fsyncDone   chan struct{}
}

// NewWAL creates a new Write-Ahead Log
func NewWAL(dir string, config WALConfig) (WAL, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	wal := &walImpl{
		config:    config,
		dir:       dir,
		segments:  make([]*walSegment, 0),
		fsyncDone: make(chan struct{}),
	}

	// Load existing segments or create new one
	if err := wal.loadSegments(); err != nil {
		return nil, err
	}

	// Start fsync ticker if needed
	if config.FsyncPolicy == FsyncInterval {
		wal.fsyncTicker = time.NewTicker(config.FsyncInterval)
		go wal.fsyncLoop()
	}

	return wal, nil
}

func (w *walImpl) Append(data []byte) (Offset, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, ErrLogClosed
	}

	// Check if we need to roll to a new segment
	if w.currentSegment == nil || w.currentSegment.size >= w.config.SegmentSize {
		if err := w.rollSegment(); err != nil {
			return 0, err
		}
	}

	// Write to current segment
	offset := w.nextOffset
	if err := w.currentSegment.append(offset, data); err != nil {
		return 0, err
	}

	w.nextOffset++

	// Fsync if policy requires it
	if w.config.FsyncPolicy == FsyncAlways {
		if err := w.currentSegment.sync(); err != nil {
			return 0, err
		}
	}

	return offset, nil
}

func (w *walImpl) Read(offset Offset) ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return nil, ErrLogClosed
	}

	// Find the segment containing this offset
	seg := w.findSegment(offset)
	if seg == nil {
		return nil, ErrOffsetOutOfRange
	}

	return seg.read(offset)
}

func (w *walImpl) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrLogClosed
	}

	if w.currentSegment != nil {
		return w.currentSegment.sync()
	}

	return nil
}

func (w *walImpl) Truncate(beforeOffset Offset) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrLogClosed
	}

	// Find segments to delete
	var toKeep []*walSegment
	for _, seg := range w.segments {
		if seg.baseOffset >= beforeOffset {
			toKeep = append(toKeep, seg)
		} else {
			seg.close()
			os.Remove(seg.path)
		}
	}

	w.segments = toKeep

	return nil
}

func (w *walImpl) NextOffset() Offset {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.nextOffset
}

func (w *walImpl) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true

	// Stop fsync loop
	if w.fsyncTicker != nil {
		w.fsyncTicker.Stop()
		close(w.fsyncDone)
	}

	// Close all segments
	for _, seg := range w.segments {
		seg.close()
	}

	return nil
}

func (w *walImpl) loadSegments() error {
	// List all segment files
	files, err := filepath.Glob(filepath.Join(w.dir, "*.wal"))
	if err != nil {
		return err
	}

	if len(files) == 0 {
		// Create first segment
		return w.rollSegment()
	}

	// Load existing segments
	for _, file := range files {
		seg, err := openWALSegment(file)
		if err != nil {
			return err
		}
		w.segments = append(w.segments, seg)
	}

	// Set current segment to last one
	w.currentSegment = w.segments[len(w.segments)-1]
	w.nextOffset = w.currentSegment.nextOffset

	return nil
}

func (w *walImpl) rollSegment() error {
	// Close current segment if exists
	if w.currentSegment != nil {
		if err := w.currentSegment.sync(); err != nil {
			return err
		}
	}

	// Create new segment
	baseOffset := w.nextOffset
	path := filepath.Join(w.dir, fmt.Sprintf("%020d.wal", baseOffset))

	seg, err := createWALSegment(path, baseOffset)
	if err != nil {
		return err
	}

	w.currentSegment = seg
	w.segments = append(w.segments, seg)

	return nil
}

func (w *walImpl) findSegment(offset Offset) *walSegment {
	// Binary search for the segment
	left, right := 0, len(w.segments)-1

	for left <= right {
		mid := (left + right) / 2
		seg := w.segments[mid]

		if offset < seg.baseOffset {
			right = mid - 1
		} else if offset >= seg.nextOffset {
			left = mid + 1
		} else {
			return seg
		}
	}

	return nil
}

func (w *walImpl) fsyncLoop() {
	for {
		select {
		case <-w.fsyncTicker.C:
			_ = w.Sync()
		case <-w.fsyncDone:
			return
		}
	}
}

// walSegment represents a single WAL segment file
type walSegment struct {
	path       string
	baseOffset Offset
	nextOffset Offset
	size       int64

	file   *os.File
	writer *bufio.Writer
	index  Index

	mu sync.RWMutex
}

// WAL record format:
// [Length: 4 bytes][Offset: 8 bytes][CRC: 4 bytes][Data: Length bytes]
const (
	recordHeaderSize = 16 // 4 + 8 + 4
)

func createWALSegment(path string, baseOffset Offset) (*walSegment, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}

	// Create index file
	indexPath := path + ".index"
	index, err := NewIndex(indexPath)
	if err != nil {
		file.Close()
		return nil, err
	}

	seg := &walSegment{
		path:       path,
		baseOffset: baseOffset,
		nextOffset: baseOffset,
		file:       file,
		writer:     bufio.NewWriter(file),
		index:      index,
	}

	return seg, nil
}

func openWALSegment(path string) (*walSegment, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	// Parse base offset from filename
	var baseOffset Offset
	_, err = fmt.Sscanf(filepath.Base(path), "%020d.wal", &baseOffset)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("invalid segment filename: %w", err)
	}

	// Open or create index
	indexPath := path + ".index"
	index, err := NewIndex(indexPath)
	if err != nil {
		file.Close()
		return nil, err
	}

	seg := &walSegment{
		path:       path,
		baseOffset: baseOffset,
		nextOffset: baseOffset,
		file:       file,
		writer:     bufio.NewWriter(file),
		index:      index,
	}

	// Scan through file to find next offset and size
	// This also rebuilds the index if needed
	if err := seg.scan(); err != nil {
		seg.close()
		return nil, err
	}

	return seg, nil
}

func (s *walSegment) append(offset Offset, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	length := uint32(len(data))
	crc := crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))

	// Record current position for index
	position := s.size

	// Write header
	header := make([]byte, recordHeaderSize)
	binary.BigEndian.PutUint32(header[0:4], length)
	binary.BigEndian.PutUint64(header[4:12], uint64(offset))
	binary.BigEndian.PutUint32(header[12:16], crc)

	if _, err := s.writer.Write(header); err != nil {
		return err
	}

	// Write data
	if _, err := s.writer.Write(data); err != nil {
		return err
	}

	s.size += int64(recordHeaderSize + length)
	s.nextOffset = offset + 1

	// Add to index (every record for now; could be sparse)
	if err := s.index.Add(offset, position); err != nil {
		return err
	}

	return nil
}

func (s *walSegment) read(offset Offset) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if offset < s.baseOffset || offset >= s.nextOffset {
		return nil, ErrOffsetOutOfRange
	}

	// Use index to find approximate position
	position, err := s.index.Lookup(offset)
	if err != nil {
		// Fall back to linear scan if index lookup fails
		position = 0
	}

	// Seek to the indexed position
	if _, err := s.file.Seek(position, 0); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, recordHeaderSize)

	// Scan from the indexed position
	for {
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF {
				return nil, ErrOffsetOutOfRange
			}
			return nil, err
		}

		length := binary.BigEndian.Uint32(header[0:4])
		recordOffset := Offset(binary.BigEndian.Uint64(header[4:12]))
		crc := binary.BigEndian.Uint32(header[12:16])

		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}

		if recordOffset == offset {
			// Verify checksum
			actualCRC := crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))
			if actualCRC != crc {
				return nil, ErrChecksumMismatch
			}
			return data, nil
		}

		// Skip ahead if we haven't reached the target offset yet
		if recordOffset > offset {
			return nil, ErrOffsetOutOfRange
		}
	}
}

func (s *walSegment) sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writer.Flush(); err != nil {
		return err
	}

	if err := s.file.Sync(); err != nil {
		return err
	}

	// Sync index
	if err := s.index.Sync(); err != nil {
		return err
	}

	return nil
}

func (s *walSegment) scan() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}

	reader := bufio.NewReader(s.file)
	header := make([]byte, recordHeaderSize)
	position := int64(0)

	for {
		recordStart := position

		n, err := io.ReadFull(reader, header)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		position += int64(n)

		length := binary.BigEndian.Uint32(header[0:4])
		offset := Offset(binary.BigEndian.Uint64(header[4:12]))

		// Rebuild index if it's empty
		if s.index.(*indexImpl).Size() == 0 {
			_ = s.index.Add(offset, recordStart)
		}

		// Skip data
		if _, err := reader.Discard(int(length)); err != nil {
			return err
		}
		position += int64(length)

		s.nextOffset = offset + 1
	}

	s.size = position

	// Sync index after rebuild
	if err := s.index.Sync(); err != nil {
		return err
	}

	return nil
}

func (s *walSegment) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writer.Flush(); err != nil {
		return err
	}

	if err := s.file.Close(); err != nil {
		return err
	}

	if err := s.index.Close(); err != nil {
		return err
	}

	return nil
}
