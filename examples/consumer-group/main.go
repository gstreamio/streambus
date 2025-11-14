package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// CustomRebalanceListener tracks partition assignments
type CustomRebalanceListener struct {
	consumerID string
}

func (l *CustomRebalanceListener) OnPartitionsRevoked(partitions map[string][]int32) {
	fmt.Printf("\n[%s] 🔄 Partitions Revoked:\n", l.consumerID)
	for topic, parts := range partitions {
		fmt.Printf("  Topic: %s, Partitions: %v\n", topic, parts)
	}
}

func (l *CustomRebalanceListener) OnPartitionsAssigned(partitions map[string][]int32) {
	fmt.Printf("\n[%s] ✓ Partitions Assigned:\n", l.consumerID)
	for topic, parts := range partitions {
		fmt.Printf("  Topic: %s, Partitions: %v\n", topic, parts)
	}
	fmt.Println()
}

func main() {
	fmt.Println("StreamBus Consumer Group Example")
	fmt.Println("=================================")

	// Get consumer ID from args or use default
	consumerID := "consumer-1"
	if len(os.Args) > 1 {
		consumerID = os.Args[1]
	}

	fmt.Printf("Consumer ID: %s\n", consumerID)

	// Create client configuration
	clientConfig := client.DefaultConfig()
	clientConfig.Brokers = []string{"localhost:9092"}

	// Create client
	c, err := client.New(clientConfig)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create group consumer configuration
	groupConfig := client.DefaultGroupConsumerConfig()
	groupConfig.GroupID = "example-consumer-group"
	groupConfig.Topics = []string{"events"}
	groupConfig.ClientID = consumerID
	groupConfig.SessionTimeoutMs = 30000     // 30 seconds
	groupConfig.HeartbeatIntervalMs = 3000   // 3 seconds
	groupConfig.RebalanceTimeoutMs = 60000   // 60 seconds
	groupConfig.AssignmentStrategy = "range" // Can be: range, roundrobin, sticky
	groupConfig.AutoCommit = false           // Manual commit for this example

	fmt.Printf("\nGroup Configuration:\n")
	fmt.Printf("  Group ID: %s\n", groupConfig.GroupID)
	fmt.Printf("  Topics: %v\n", groupConfig.Topics)
	fmt.Printf("  Assignment Strategy: %s\n", groupConfig.AssignmentStrategy)
	fmt.Printf("  Auto Commit: %v\n", groupConfig.AutoCommit)

	// Create group consumer
	gc, err := client.NewGroupConsumer(c, groupConfig)
	if err != nil {
		log.Fatalf("Failed to create group consumer: %v", err)
	}
	defer gc.Close()

	// Set custom rebalance listener
	listener := &CustomRebalanceListener{consumerID: consumerID}
	gc.SetRebalanceListener(listener)

	// Subscribe to the group
	fmt.Println("\n📡 Subscribing to consumer group...")
	ctx := context.Background()
	if err := gc.Subscribe(ctx); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	fmt.Println("✓ Successfully joined consumer group")
	fmt.Println("\nStarting to consume messages (Press Ctrl+C to stop)...")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Message processing loop
	messageCount := 0
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Commit ticker for periodic offset commits
	commitTicker := time.NewTicker(10 * time.Second)
	defer commitTicker.Stop()

	// Track offsets for commit
	offsetsToCommit := make(map[string]map[int32]int64)

	for {
		select {
		case <-sigChan:
			fmt.Println("\n\n🛑 Received shutdown signal...")

			// Commit any pending offsets
			if len(offsetsToCommit) > 0 {
				fmt.Println("💾 Committing final offsets...")
				if err := gc.CommitSync(ctx, offsetsToCommit); err != nil {
					log.Printf("Failed to commit offsets: %v", err)
				}
			}

			// Print final stats
			printStats(gc, messageCount)
			fmt.Println("\n✓ Consumer group example completed successfully!")
			return

		case <-ticker.C:
			// Periodic status update
			assignment := gc.Assignment()
			fmt.Printf("\n📊 Status Update - Messages processed: %d, Assigned partitions: %v\n",
				messageCount, assignment)

		case <-commitTicker.C:
			// Commit offsets periodically
			if len(offsetsToCommit) > 0 {
				fmt.Printf("\n💾 Committing offsets...\n")
				if err := gc.CommitSync(ctx, offsetsToCommit); err != nil {
					log.Printf("Failed to commit offsets: %v", err)
				} else {
					fmt.Printf("✓ Committed offsets for %d topics\n", len(offsetsToCommit))
					// Clear committed offsets
					offsetsToCommit = make(map[string]map[int32]int64)
				}
			}

		default:
			// Poll for messages
			messages, err := gc.Poll(pollCtx)
			if err != nil {
				log.Printf("Poll error: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// Process messages
			for topic, partitions := range messages {
				for partition, msgs := range partitions {
					for _, msg := range msgs {
						messageCount++
						fmt.Printf("\n📨 Message #%d:\n", messageCount)
						fmt.Printf("  Topic:     %s\n", topic)
						fmt.Printf("  Partition: %d\n", partition)
						fmt.Printf("  Offset:    %d\n", msg.Offset)
						fmt.Printf("  Key:       %s\n", string(msg.Key))
						fmt.Printf("  Value:     %s\n", string(msg.Value))
						fmt.Printf("  Timestamp: %s\n", time.Unix(0, msg.Timestamp).Format(time.RFC3339))

						// Track offset for commit (offset + 1 is the next message to consume)
						if offsetsToCommit[topic] == nil {
							offsetsToCommit[topic] = make(map[int32]int64)
						}
						offsetsToCommit[topic][partition] = msg.Offset + 1
					}
				}
			}

			// Small delay between polls
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func printStats(gc *client.GroupConsumer, messageCount int) {
	stats := gc.Stats()
	fmt.Println("\n=================================")
	fmt.Println("Consumer Group Statistics:")
	fmt.Println("=================================")
	fmt.Printf("  Group ID:         %s\n", stats.GroupID)
	fmt.Printf("  Member ID:        %s\n", stats.MemberID)
	fmt.Printf("  State:            %v\n", stats.State)
	fmt.Printf("  Messages Read:    %d\n", messageCount)
	fmt.Printf("  Rebalance Count:  %d\n", stats.RebalanceCount)
	fmt.Printf("  Offsets Committed: %d\n", stats.OffsetsCommitted)
	fmt.Println("=================================")
}
