package protocol

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// Codec handles encoding and decoding of protocol messages
type Codec struct {
	maxMessageSize uint32
}

// NewCodec creates a new codec
func NewCodec() *Codec {
	return &Codec{
		maxMessageSize: MaxMessageSize,
	}
}

// EncodeRequest encodes a request to the writer
func (c *Codec) EncodeRequest(w io.Writer, req *Request) error {
	// Calculate payload size
	payloadSize, err := c.calculateRequestPayloadSize(req)
	if err != nil {
		return err
	}

	// Total size = Header + Payload + CRC
	totalSize := HeaderSize + payloadSize

	if totalSize > c.maxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max %d)", totalSize, c.maxMessageSize)
	}

	// Create buffer for the entire message
	buf := make([]byte, totalSize)
	offset := 0

	// Write header
	binary.BigEndian.PutUint32(buf[offset:], uint32(totalSize-4)) // Length excludes length field itself
	offset += 4
	binary.BigEndian.PutUint64(buf[offset:], req.Header.RequestID)
	offset += 8
	buf[offset] = byte(req.Header.Type)
	offset++
	buf[offset] = req.Header.Version
	offset++
	binary.BigEndian.PutUint16(buf[offset:], uint16(req.Header.Flags))
	offset += 2

	// Write payload
	offset, err = c.encodeRequestPayload(buf, offset, req)
	if err != nil {
		return err
	}

	// Calculate and write CRC32 (checksum of header + payload, excluding length field)
	crc := crc32.ChecksumIEEE(buf[4:offset]) // Start after length field
	binary.BigEndian.PutUint32(buf[offset:], crc)
	offset += 4

	// Write to writer
	_, err = w.Write(buf[:offset])
	return err
}

// DecodeRequest decodes a request from the reader
func (c *Codec) DecodeRequest(r io.Reader) (*Request, error) {
	// Read length
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	if length > c.maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, c.maxMessageSize)
	}

	// Read rest of message
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	// Verify CRC (last 4 bytes)
	if length < 4 {
		return nil, fmt.Errorf("message too short for CRC")
	}

	receivedCRC := binary.BigEndian.Uint32(buf[length-4:])
	calculatedCRC := crc32.ChecksumIEEE(buf[:length-4])
	if receivedCRC != calculatedCRC {
		return nil, ErrChecksumMismatch
	}

	// Parse header
	offset := 0
	req := &Request{}
	req.Header.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8
	req.Header.Type = RequestType(buf[offset])
	offset++
	req.Header.Version = buf[offset]
	offset++
	req.Header.Flags = RequestFlags(binary.BigEndian.Uint16(buf[offset:]))
	offset += 2

	// Parse payload
	payload, err := c.decodeRequestPayload(buf[offset:length-4], req.Header.Type)
	if err != nil {
		return nil, err
	}
	req.Payload = payload

	return req, nil
}

// EncodeResponse encodes a response to the writer
func (c *Codec) EncodeResponse(w io.Writer, resp *Response) error {
	// Calculate payload size
	payloadSize, err := c.calculateResponsePayloadSize(resp)
	if err != nil {
		return err
	}

	// Total size = Header + Payload + CRC
	totalSize := 19 + payloadSize // Length(4) + RequestID(8) + Status(1) + ErrorCode(2) + Payload + CRC(4)

	if totalSize > c.maxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max %d)", totalSize, c.maxMessageSize)
	}

	// Create buffer
	buf := make([]byte, totalSize)
	offset := 0

	// Write header
	binary.BigEndian.PutUint32(buf[offset:], uint32(totalSize-4))
	offset += 4
	binary.BigEndian.PutUint64(buf[offset:], resp.Header.RequestID)
	offset += 8
	buf[offset] = byte(resp.Header.Status)
	offset++
	binary.BigEndian.PutUint16(buf[offset:], uint16(resp.Header.ErrorCode))
	offset += 2

	// Write payload
	offset, err = c.encodeResponsePayload(buf, offset, resp)
	if err != nil {
		return err
	}

	// Calculate and write CRC32 (checksum excluding length field)
	crc := crc32.ChecksumIEEE(buf[4:offset]) // Start after length field
	binary.BigEndian.PutUint32(buf[offset:], crc)
	offset += 4

	// Write to writer
	_, err = w.Write(buf[:offset])
	return err
}

// DecodeResponse decodes a response from the reader
func (c *Codec) DecodeResponse(r io.Reader) (*Response, error) {
	// Read length
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	if length > c.maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, c.maxMessageSize)
	}

	// Read rest of message
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	// Verify CRC
	if length < 4 {
		return nil, fmt.Errorf("message too short for CRC")
	}

	receivedCRC := binary.BigEndian.Uint32(buf[length-4:])
	calculatedCRC := crc32.ChecksumIEEE(buf[:length-4])
	if receivedCRC != calculatedCRC {
		return nil, ErrChecksumMismatch
	}

	// Parse header
	offset := 0
	resp := &Response{}
	resp.Header.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8
	resp.Header.Status = StatusCode(buf[offset])
	offset++
	resp.Header.ErrorCode = ErrorCode(binary.BigEndian.Uint16(buf[offset:]))
	offset += 2

	// Parse payload based on status
	if resp.Header.Status != StatusOK {
		// Error response
		errorResp := &ErrorResponse{
			ErrorCode: resp.Header.ErrorCode,
		}
		// Read error message if present
		if offset < int(length)-4 {
			msgLen := binary.BigEndian.Uint32(buf[offset:])
			offset += 4
			if offset+int(msgLen) <= int(length)-4 {
				errorResp.Message = string(buf[offset : offset+int(msgLen)])
			}
		}
		resp.Payload = errorResp
	} else {
		// Success response - store raw bytes for later decoding
		resp.Payload = buf[offset : length-4]
	}

	return resp, nil
}

// DecodeResponsePayload decodes a response payload based on request type
func (c *Codec) DecodeResponsePayload(resp *Response, reqType RequestType) error {
	data, ok := resp.Payload.([]byte)
	if !ok {
		// Already decoded
		return nil
	}

	offset := 0
	switch reqType {
	case RequestTypeProduce:
		payload := &ProduceResponse{}
		payload.BaseOffset = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		payload.NumMessages = binary.BigEndian.Uint32(data[offset:])
		offset += 4
		payload.HighWaterMark = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		resp.Payload = payload
		return nil

	case RequestTypeFetch:
		payload := &FetchResponse{}
		payload.HighWaterMark = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		numMessages := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		payload.Messages = make([]Message, numMessages)
		for i := uint32(0); i < numMessages; i++ {
			payload.Messages[i], offset = c.decodeMessage(data, offset)
		}
		resp.Payload = payload
		return nil

	case RequestTypeGetOffset:
		payload := &GetOffsetResponse{}
		payload.StartOffset = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		payload.EndOffset = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		payload.HighWaterMark = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		resp.Payload = payload
		return nil

	case RequestTypeCreateTopic:
		payload := &CreateTopicResponse{}
		topicLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		payload.Topic = string(data[offset : offset+int(topicLen)])
		offset += int(topicLen)
		payload.Created = data[offset] == 1
		offset++
		resp.Payload = payload
		return nil

	case RequestTypeDeleteTopic:
		payload := &DeleteTopicResponse{}
		topicLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		payload.Topic = string(data[offset : offset+int(topicLen)])
		offset += int(topicLen)
		payload.Deleted = data[offset] == 1
		offset++
		resp.Payload = payload
		return nil

	case RequestTypeListTopics:
		payload := &ListTopicsResponse{}
		numTopics := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		payload.Topics = make([]TopicInfo, numTopics)
		for i := uint32(0); i < numTopics; i++ {
			nameLen := binary.BigEndian.Uint32(data[offset:])
			offset += 4
			payload.Topics[i].Name = string(data[offset : offset+int(nameLen)])
			offset += int(nameLen)
			payload.Topics[i].NumPartitions = binary.BigEndian.Uint32(data[offset:])
			offset += 4
		}
		resp.Payload = payload
		return nil

	case RequestTypeHealthCheck:
		payload := &HealthCheckResponse{}
		statusLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		payload.Status = string(data[offset : offset+int(statusLen)])
		offset += int(statusLen)
		payload.Uptime = int64(binary.BigEndian.Uint64(data[offset:]))
		offset += 8
		resp.Payload = payload
		return nil

	default:
		return fmt.Errorf("unknown request type: %v", reqType)
	}
}

// Helper methods for calculating sizes

func (c *Codec) calculateRequestPayloadSize(req *Request) (uint32, error) {
	switch req.Header.Type {
	case RequestTypeProduce:
		payload := req.Payload.(*ProduceRequest)
		size := 4 + len(payload.Topic) + 4 // TopicLen + Topic + PartitionID
		size += 4                           // NumMessages
		for _, msg := range payload.Messages {
			size += msg.Size()
		}
		return uint32(size), nil

	case RequestTypeFetch:
		payload := req.Payload.(*FetchRequest)
		size := 4 + len(payload.Topic) + 4 + 8 + 4 // TopicLen + Topic + PartitionID + Offset + MaxBytes
		return uint32(size), nil

	case RequestTypeGetOffset:
		payload := req.Payload.(*GetOffsetRequest)
		size := 4 + len(payload.Topic) + 4 // TopicLen + Topic + PartitionID
		return uint32(size), nil

	case RequestTypeCreateTopic:
		payload := req.Payload.(*CreateTopicRequest)
		size := 4 + len(payload.Topic) + 4 + 2 // TopicLen + Topic + NumPartitions + ReplicationFactor
		return uint32(size), nil

	case RequestTypeDeleteTopic:
		payload := req.Payload.(*DeleteTopicRequest)
		size := 4 + len(payload.Topic) // TopicLen + Topic
		return uint32(size), nil

	case RequestTypeListTopics, RequestTypeHealthCheck:
		return 0, nil

	default:
		return 0, fmt.Errorf("unknown request type: %v", req.Header.Type)
	}
}

func (c *Codec) calculateResponsePayloadSize(resp *Response) (uint32, error) {
	if resp.Header.Status != StatusOK {
		errorResp := resp.Payload.(*ErrorResponse)
		return uint32(4 + len(errorResp.Message)), nil // MsgLen + Message
	}

	// For success responses, size depends on response type
	switch payload := resp.Payload.(type) {
	case *ProduceResponse:
		return 8 + 4 + 8, nil // BaseOffset + NumMessages + HighWaterMark

	case *FetchResponse:
		size := uint32(8 + 4) // HighWaterMark + NumMessages
		for _, msg := range payload.Messages {
			size += uint32(msg.Size())
		}
		return size, nil

	case *GetOffsetResponse:
		return 8 + 8 + 8, nil // StartOffset + EndOffset + HighWaterMark

	case *CreateTopicResponse:
		return uint32(4 + len(payload.Topic) + 1), nil // TopicLen + Topic + Created

	case *DeleteTopicResponse:
		return uint32(4 + len(payload.Topic) + 1), nil // TopicLen + Topic + Deleted

	case *ListTopicsResponse:
		size := uint32(4) // NumTopics
		for _, topic := range payload.Topics {
			size += uint32(4 + len(topic.Name) + 4) // NameLen + Name + NumPartitions
		}
		return size, nil

	case *HealthCheckResponse:
		return uint32(4 + len(payload.Status) + 8), nil // StatusLen + Status + Uptime

	case []byte:
		return uint32(len(payload)), nil

	default:
		return 0, nil
	}
}

// encodeRequestPayload encodes the request payload
func (c *Codec) encodeRequestPayload(buf []byte, offset int, req *Request) (int, error) {
	switch req.Header.Type {
	case RequestTypeProduce:
		payload := req.Payload.(*ProduceRequest)
		// Topic
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		// PartitionID
		binary.BigEndian.PutUint32(buf[offset:], payload.PartitionID)
		offset += 4
		// NumMessages
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Messages)))
		offset += 4
		// Messages
		for _, msg := range payload.Messages {
			offset = c.encodeMessage(buf, offset, &msg)
		}
		return offset, nil

	case RequestTypeFetch:
		payload := req.Payload.(*FetchRequest)
		// Topic
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		// PartitionID
		binary.BigEndian.PutUint32(buf[offset:], payload.PartitionID)
		offset += 4
		// Offset
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.Offset))
		offset += 8
		// MaxBytes
		binary.BigEndian.PutUint32(buf[offset:], payload.MaxBytes)
		offset += 4
		return offset, nil

	case RequestTypeGetOffset:
		payload := req.Payload.(*GetOffsetRequest)
		// Topic
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		// PartitionID
		binary.BigEndian.PutUint32(buf[offset:], payload.PartitionID)
		offset += 4
		return offset, nil

	case RequestTypeCreateTopic:
		payload := req.Payload.(*CreateTopicRequest)
		// Topic
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		// NumPartitions
		binary.BigEndian.PutUint32(buf[offset:], payload.NumPartitions)
		offset += 4
		// ReplicationFactor
		binary.BigEndian.PutUint16(buf[offset:], payload.ReplicationFactor)
		offset += 2
		return offset, nil

	case RequestTypeDeleteTopic:
		payload := req.Payload.(*DeleteTopicRequest)
		// Topic
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		return offset, nil

	case RequestTypeListTopics, RequestTypeHealthCheck:
		// No payload
		return offset, nil

	default:
		return offset, fmt.Errorf("unknown request type: %v", req.Header.Type)
	}
}

// decodeRequestPayload decodes the request payload
func (c *Codec) decodeRequestPayload(buf []byte, reqType RequestType) (interface{}, error) {
	offset := 0

	switch reqType {
	case RequestTypeProduce:
		// Topic
		topicLen := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		topic := string(buf[offset : offset+int(topicLen)])
		offset += int(topicLen)
		// PartitionID
		partitionID := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		// NumMessages
		numMessages := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		// Messages
		messages := make([]Message, numMessages)
		for i := uint32(0); i < numMessages; i++ {
			msg, newOffset := c.decodeMessage(buf, offset)
			messages[i] = msg
			offset = newOffset
		}
		return &ProduceRequest{
			Topic:       topic,
			PartitionID: partitionID,
			Messages:    messages,
		}, nil

	case RequestTypeFetch:
		// Topic
		topicLen := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		topic := string(buf[offset : offset+int(topicLen)])
		offset += int(topicLen)
		// PartitionID
		partitionID := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		// Offset
		fetchOffset := int64(binary.BigEndian.Uint64(buf[offset:]))
		offset += 8
		// MaxBytes
		maxBytes := binary.BigEndian.Uint32(buf[offset:])
		return &FetchRequest{
			Topic:       topic,
			PartitionID: partitionID,
			Offset:      fetchOffset,
			MaxBytes:    maxBytes,
		}, nil

	case RequestTypeGetOffset:
		// Topic
		topicLen := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		topic := string(buf[offset : offset+int(topicLen)])
		offset += int(topicLen)
		// PartitionID
		partitionID := binary.BigEndian.Uint32(buf[offset:])
		return &GetOffsetRequest{
			Topic:       topic,
			PartitionID: partitionID,
		}, nil

	case RequestTypeCreateTopic:
		// Topic
		topicLen := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		topic := string(buf[offset : offset+int(topicLen)])
		offset += int(topicLen)
		// NumPartitions
		numPartitions := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		// ReplicationFactor
		replicationFactor := binary.BigEndian.Uint16(buf[offset:])
		return &CreateTopicRequest{
			Topic:             topic,
			NumPartitions:     numPartitions,
			ReplicationFactor: replicationFactor,
		}, nil

	case RequestTypeDeleteTopic:
		// Topic
		topicLen := binary.BigEndian.Uint32(buf[offset:])
		offset += 4
		topic := string(buf[offset : offset+int(topicLen)])
		return &DeleteTopicRequest{
			Topic: topic,
		}, nil

	case RequestTypeListTopics:
		return &ListTopicsRequest{}, nil

	case RequestTypeHealthCheck:
		return &HealthCheckRequest{}, nil

	default:
		return nil, fmt.Errorf("unknown request type: %v", reqType)
	}
}

// encodeResponsePayload encodes the response payload
func (c *Codec) encodeResponsePayload(buf []byte, offset int, resp *Response) (int, error) {
	if resp.Header.Status != StatusOK {
		errorResp := resp.Payload.(*ErrorResponse)
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(errorResp.Message)))
		offset += 4
		copy(buf[offset:], errorResp.Message)
		offset += len(errorResp.Message)
		return offset, nil
	}

	// For success responses, encode based on type
	switch payload := resp.Payload.(type) {
	case *ProduceResponse:
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.BaseOffset))
		offset += 8
		binary.BigEndian.PutUint32(buf[offset:], payload.NumMessages)
		offset += 4
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.HighWaterMark))
		offset += 8
		return offset, nil

	case *FetchResponse:
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.HighWaterMark))
		offset += 8
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Messages)))
		offset += 4
		for _, msg := range payload.Messages {
			offset = c.encodeMessage(buf, offset, &msg)
		}
		return offset, nil

	case *GetOffsetResponse:
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.StartOffset))
		offset += 8
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.EndOffset))
		offset += 8
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.HighWaterMark))
		offset += 8
		return offset, nil

	case *CreateTopicResponse:
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		if payload.Created {
			buf[offset] = 1
		} else {
			buf[offset] = 0
		}
		offset++
		return offset, nil

	case *DeleteTopicResponse:
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topic)))
		offset += 4
		copy(buf[offset:], payload.Topic)
		offset += len(payload.Topic)
		if payload.Deleted {
			buf[offset] = 1
		} else {
			buf[offset] = 0
		}
		offset++
		return offset, nil

	case *ListTopicsResponse:
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Topics)))
		offset += 4
		for _, topic := range payload.Topics {
			binary.BigEndian.PutUint32(buf[offset:], uint32(len(topic.Name)))
			offset += 4
			copy(buf[offset:], topic.Name)
			offset += len(topic.Name)
			binary.BigEndian.PutUint32(buf[offset:], topic.NumPartitions)
			offset += 4
		}
		return offset, nil

	case *HealthCheckResponse:
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(payload.Status)))
		offset += 4
		copy(buf[offset:], payload.Status)
		offset += len(payload.Status)
		binary.BigEndian.PutUint64(buf[offset:], uint64(payload.Uptime))
		offset += 8
		return offset, nil

	case []byte:
		copy(buf[offset:], payload)
		offset += len(payload)
		return offset, nil

	default:
		return offset, nil
	}
}

// encodeMessage encodes a single message
func (c *Codec) encodeMessage(buf []byte, offset int, msg *Message) int {
	// Offset
	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Offset))
	offset += 8
	// Timestamp
	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Timestamp))
	offset += 8
	// Key
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Key)))
	offset += 4
	if len(msg.Key) > 0 {
		copy(buf[offset:], msg.Key)
		offset += len(msg.Key)
	}
	// Value
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Value)))
	offset += 4
	if len(msg.Value) > 0 {
		copy(buf[offset:], msg.Value)
		offset += len(msg.Value)
	}
	// Headers
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Headers)))
	offset += 4
	for k, v := range msg.Headers {
		// Header key
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(k)))
		offset += 4
		copy(buf[offset:], k)
		offset += len(k)
		// Header value
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(v)))
		offset += 4
		copy(buf[offset:], v)
		offset += len(v)
	}
	return offset
}

// decodeMessage decodes a single message
func (c *Codec) decodeMessage(buf []byte, offset int) (Message, int) {
	msg := Message{}
	// Offset
	msg.Offset = int64(binary.BigEndian.Uint64(buf[offset:]))
	offset += 8
	// Timestamp
	msg.Timestamp = int64(binary.BigEndian.Uint64(buf[offset:]))
	offset += 8
	// Key
	keyLen := binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	if keyLen > 0 {
		msg.Key = make([]byte, keyLen)
		copy(msg.Key, buf[offset:offset+int(keyLen)])
		offset += int(keyLen)
	}
	// Value
	valueLen := binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	if valueLen > 0 {
		msg.Value = make([]byte, valueLen)
		copy(msg.Value, buf[offset:offset+int(valueLen)])
		offset += int(valueLen)
	}
	// Headers
	numHeaders := binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	if numHeaders > 0 {
		msg.Headers = make(map[string][]byte)
		for i := uint32(0); i < numHeaders; i++ {
			// Header key
			hkLen := binary.BigEndian.Uint32(buf[offset:])
			offset += 4
			hk := string(buf[offset : offset+int(hkLen)])
			offset += int(hkLen)
			// Header value
			hvLen := binary.BigEndian.Uint32(buf[offset:])
			offset += 4
			hv := make([]byte, hvLen)
			copy(hv, buf[offset:offset+int(hvLen)])
			offset += int(hvLen)
			msg.Headers[hk] = hv
		}
	}
	return msg, offset
}
