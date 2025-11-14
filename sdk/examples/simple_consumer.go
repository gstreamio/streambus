package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gstreamio/streambus-sdk/streambus"
)

func main() {
	// Example 1: Simple consumer
	simpleConsumer()

	// Example 2: Consumer with auto-commit
	consumerWithAutoCommit()

	// Example 3: JSON consumer
	jsonConsumer()

	// Example 4: Consumer group
	consumerGroup()
}

func simpleConsumer() {
	fmt.Println("\n=== Simple Consumer Example ===")

	// Connect to StreamBus
	client, err := streambus.Connect("localhost:9092")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create a consumer for a topic
	consumer := client.NewConsumer("my-topic")
	defer consumer.Close()

	// Consume 5 messages
	err = consumer.ConsumeN(5, func(msg *streambus.ReceivedMessage) error {
		fmt.Printf("Received: %s (offset: %d)\n", string(msg.Value), msg.Offset)
		return nil
	})

	if err != nil {
		log.Printf("Error consuming: %v", err)
	}
}

func consumerWithAutoCommit() {
	fmt.Println("\n=== Consumer with Auto-Commit Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	// Create consumer with builder pattern
	consumer := client.NewConsumerBuilder("orders").
		WithAutoCommit().
		WithRetries(3).
		Build()
	defer consumer.Close()

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down consumer...")
		cancel()
	}()

	// Consume messages with auto-commit
	err := consumer.Consume(ctx, func(msg *streambus.ReceivedMessage) error {
		fmt.Printf("Processing order: %s\n", string(msg.Value))

		// Simulate processing
		time.Sleep(100 * time.Millisecond)

		// Auto-commit happens automatically after successful processing
		return nil
	})

	if err != nil && err != context.Canceled {
		log.Printf("Consumer error: %v", err)
	}
}

func jsonConsumer() {
	fmt.Println("\n=== JSON Consumer Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	consumer := client.NewConsumer("events")
	defer consumer.Close()

	// Define the expected data structure
	type Event struct {
		ID        string                 `json:"id"`
		Type      string                 `json:"type"`
		Timestamp int64                  `json:"timestamp"`
		Data      map[string]interface{} `json:"data"`
	}

	// Consume JSON messages
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var event Event
	err := consumer.ConsumeJSON(ctx, func(msg *streambus.ReceivedMessage, data interface{}) error {
		evt := data.(*Event)
		fmt.Printf("Event received:\n")
		fmt.Printf("  ID: %s\n", evt.ID)
		fmt.Printf("  Type: %s\n", evt.Type)
		fmt.Printf("  Data: %+v\n", evt.Data)
		return nil
	}, &event)

	if err != nil && err != context.DeadlineExceeded {
		log.Printf("JSON consumer error: %v", err)
	}
}

func consumerGroup() {
	fmt.Println("\n=== Consumer Group Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	// Create a consumer group
	group := client.NewConsumerGroupBuilder("my-group").
		WithTopics("topic1", "topic2", "topic3").
		WithAutoCommit(5 * time.Second).
		WithSessionTimeout(30 * time.Second).
		Build()

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nStopping consumer group...")
		cancel()
	}()

	// Start the consumer group
	fmt.Println("Starting consumer group (press Ctrl+C to stop)...")
	err := group.Start(ctx, func(msg *streambus.ReceivedMessage) error {
		fmt.Printf("[%s] Message: %s\n", msg.Topic, string(msg.Value))
		return nil
	})

	if err != nil && err != context.Canceled {
		log.Printf("Consumer group error: %v", err)
	}
}

// Advanced example: Consumer with error handling and retries
func advancedConsumer() {
	fmt.Println("\n=== Advanced Consumer Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	consumer := client.NewConsumer("critical-events")
	defer consumer.Close()

	// Seek to beginning to reprocess all messages
	consumer.SeekToBeginning()

	ctx := context.Background()

	err := consumer.Consume(ctx, func(msg *streambus.ReceivedMessage) error {
		// Parse the message
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Value, &data); err != nil {
			// Log error but continue - don't fail on bad messages
			log.Printf("Failed to parse message at offset %d: %v", msg.Offset, err)
			return nil
		}

		// Process based on event type
		eventType, ok := data["type"].(string)
		if !ok {
			log.Printf("Message missing type field at offset %d", msg.Offset)
			return nil
		}

		switch eventType {
		case "payment":
			return processPayment(data)
		case "order":
			return processOrder(data)
		default:
			log.Printf("Unknown event type: %s", eventType)
		}

		return nil
	})

	if err != nil {
		log.Printf("Consumer failed: %v", err)
	}
}

func processPayment(data map[string]interface{}) error {
	fmt.Printf("Processing payment: %+v\n", data)
	// Add your payment processing logic here
	return nil
}

func processOrder(data map[string]interface{}) error {
	fmt.Printf("Processing order: %+v\n", data)
	// Add your order processing logic here
	return nil
}