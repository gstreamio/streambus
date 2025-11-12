package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
)

func main() {
	fmt.Println("StreamBus Producer Example")
	fmt.Println("==========================")

	// Create client configuration
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	// Create client
	c, err := client.New(config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create topic
	fmt.Println("\nCreating topic 'events'...")
	err = c.CreateTopic("events", 3, 1) // 3 partitions, replication factor 1
	if err != nil {
		log.Printf("Topic creation failed (may already exist): %v", err)
	}

	// Create producer
	producer := client.NewProducer(c)
	defer producer.Close()

	fmt.Println("\nProducing messages...")

	// Send messages
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("message-%d: Hello from StreamBus at %s", i, time.Now().Format(time.RFC3339)))

		err := producer.Send("events", key, value)
		if err != nil {
			log.Printf("Failed to send message %d: %v", i, err)
			continue
		}

		fmt.Printf("✓ Sent message %d\n", i)
		time.Sleep(100 * time.Millisecond)
	}

	// Flush any remaining batched messages
	fmt.Println("\nFlushing producer...")
	err = producer.FlushAll()
	if err != nil {
		log.Printf("Failed to flush: %v", err)
	}

	// Get producer stats
	stats := producer.Stats()
	fmt.Println("\nProducer Statistics:")
	fmt.Printf("  Messages Sent:   %d\n", stats.MessagesSent)
	fmt.Printf("  Messages Failed: %d\n", stats.MessagesFailed)
	fmt.Printf("  Batches Sent:    %d\n", stats.BatchesSent)

	fmt.Println("\n✓ Producer example completed successfully!")
}
