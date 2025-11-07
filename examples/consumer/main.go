package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

func main() {
	fmt.Println("StreamBus Consumer Example")
	fmt.Println("==========================")

	// Create client configuration
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}
	config.ConsumerConfig.StartOffset = 0 // Start from beginning

	// Create client
	c, err := client.New(config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create consumer for partition 0 of "events" topic
	consumer := client.NewConsumer(c, "events", 0)
	defer consumer.Close()

	// Seek to beginning to read all messages
	fmt.Println("\nSeeking to beginning of partition 0...")
	err = consumer.SeekToBeginning()
	if err != nil {
		log.Fatalf("Failed to seek: %v", err)
	}

	fmt.Println("Consuming messages (Press Ctrl+C to stop)...")
	fmt.Println()

	messageCount := 0
	emptyFetchCount := 0
	maxEmptyFetches := 3

	// Consume messages
	for {
		messages, err := consumer.Fetch()
		if err != nil {
			log.Printf("Fetch error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(messages) == 0 {
			emptyFetchCount++
			if emptyFetchCount >= maxEmptyFetches {
				fmt.Println("\nNo more messages available.")
				break
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Reset empty fetch counter
		emptyFetchCount = 0

		// Process messages
		for _, msg := range messages {
			messageCount++
			fmt.Printf("Message #%d:\n", messageCount)
			fmt.Printf("  Offset:    %d\n", msg.Offset)
			fmt.Printf("  Key:       %s\n", string(msg.Key))
			fmt.Printf("  Value:     %s\n", string(msg.Value))
			fmt.Printf("  Timestamp: %s\n", time.Unix(0, msg.Timestamp).Format(time.RFC3339))
			fmt.Println()
		}

		// Small delay between fetches
		time.Sleep(100 * time.Millisecond)
	}

	// Get consumer stats
	stats := consumer.Stats()
	fmt.Println("\nConsumer Statistics:")
	fmt.Printf("  Messages Read: %d\n", stats.MessagesRead)
	fmt.Printf("  Bytes Read:    %d\n", stats.BytesRead)
	fmt.Printf("  Fetch Count:   %d\n", stats.FetchCount)

	fmt.Println("\n✓ Consumer example completed successfully!")
}
