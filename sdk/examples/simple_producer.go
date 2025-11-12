package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gstreamio/streambus-sdk/streambus"
)

func main() {
	// Example 1: Simplest possible producer
	simpleProducer()

	// Example 2: Producer with key-value pairs
	producerWithKeys()

	// Example 3: JSON producer
	jsonProducer()

	// Example 4: Batch producer
	batchProducer()

	// Example 5: Async producer
	asyncProducer()
}

func simpleProducer() {
	fmt.Println("\n=== Simple Producer Example ===")

	// Connect to StreamBus with one line
	client, err := streambus.Connect("localhost:9092")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create a producer
	producer := client.NewProducer()
	defer producer.Close()

	// Send a simple message
	if err := producer.Send("my-topic", "Hello, StreamBus!"); err != nil {
		log.Printf("Failed to send: %v", err)
	} else {
		fmt.Println("Message sent successfully!")
	}
}

func producerWithKeys() {
	fmt.Println("\n=== Producer with Keys Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	producer := client.NewProducer()
	defer producer.Close()

	// Send messages with keys for ordering
	// Messages with the same key will be ordered
	orders := []struct{ key, value string }{
		{"order-123", "Order created for customer ABC"},
		{"order-124", "Order created for customer XYZ"},
		{"order-123", "Order updated - added item"},
		{"order-123", "Order completed"},
	}

	for _, order := range orders {
		if err := producer.SendWithKey("orders", order.key, order.value); err != nil {
			log.Printf("Failed to send order %s: %v", order.key, err)
		}
	}

	// Ensure all messages are sent
	producer.FlushAll()
	fmt.Println("All orders sent!")
}

func jsonProducer() {
	fmt.Println("\n=== JSON Producer Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	producer := client.NewProducer()
	defer producer.Close()

	// Define a struct for your data
	type Event struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Timestamp int64  `json:"timestamp"`
		Data      map[string]interface{} `json:"data"`
	}

	// Send JSON data
	event := Event{
		ID:        "evt-001",
		Type:      "user.signup",
		Timestamp: 1234567890,
		Data: map[string]interface{}{
			"user_id": "usr-123",
			"email":   "user@example.com",
			"plan":    "premium",
		},
	}

	if err := producer.SendJSON("events", event.ID, event); err != nil {
		log.Printf("Failed to send JSON: %v", err)
	} else {
		fmt.Println("JSON event sent!")
	}
}

func batchProducer() {
	fmt.Println("\n=== Batch Producer Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	producer := client.NewProducer()
	defer producer.Close()

	// Prepare a batch of messages
	messages := []streambus.Message{
		{Key: []byte("msg-1"), Value: []byte("First message")},
		{Key: []byte("msg-2"), Value: []byte("Second message")},
		{Key: []byte("msg-3"), Value: []byte("Third message")},
		{Key: []byte("msg-4"), Value: []byte("Fourth message")},
		{Key: []byte("msg-5"), Value: []byte("Fifth message")},
	}

	// Send all messages in one batch
	if err := producer.SendBatch("batch-topic", messages); err != nil {
		log.Printf("Failed to send batch: %v", err)
	} else {
		fmt.Printf("Batch of %d messages sent!\n", len(messages))
	}
}

func asyncProducer() {
	fmt.Println("\n=== Async Producer Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	producer := client.NewProducer()
	defer producer.Close()

	// Send messages asynchronously
	ctx := context.Background()
	errChan := producer.SendAsync(ctx, "async-topic", "Async message")

	// Do other work while message is being sent
	fmt.Println("Doing other work...")

	// Check if send succeeded
	if err := <-errChan; err != nil {
		log.Printf("Async send failed: %v", err)
	} else {
		fmt.Println("Async message sent!")
	}
}

// Example using the fluent builder pattern
func builderExample() {
	fmt.Println("\n=== Builder Pattern Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	// Create producer with fluent builder
	producer := client.NewProducerBuilder().
		WithTopic("default-topic").
		WithPartition(0).
		Build()
	defer producer.Close()

	// Now you can send without specifying topic
	producer.Send("", "Message to default topic")
}