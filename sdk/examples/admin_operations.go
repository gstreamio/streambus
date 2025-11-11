package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shawntherrien/streambus-sdk/streambus"
)

func main() {
	// Example 1: Topic management
	topicManagement()

	// Example 2: Cluster inspection
	clusterInspection()

	// Example 3: Consumer group management
	consumerGroupManagement()
}

func topicManagement() {
	fmt.Println("\n=== Topic Management Example ===")

	// Connect to StreamBus
	client, err := streambus.Connect("localhost:9092")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Get admin client
	admin := client.Admin()

	// Create a simple topic
	fmt.Println("Creating topic 'user-events'...")
	err = admin.QuickCreateTopic("user-events", 10, 3)
	if err != nil {
		log.Printf("Failed to create topic: %v", err)
	} else {
		fmt.Println("Topic created successfully!")
	}

	// Create topic with full configuration
	fmt.Println("\nCreating topic with custom config...")
	topicConfig := streambus.TopicConfig{
		Name:              "analytics-events",
		Partitions:        20,
		ReplicationFactor: 3,
		RetentionTime:     7 * 24 * time.Hour, // 7 days
		RetentionBytes:    1024 * 1024 * 1024, // 1GB
		SegmentSize:       100 * 1024 * 1024,  // 100MB
		MinInSyncReplicas: 2,
		CompressionType:   "snappy",
		MaxMessageBytes:   1024 * 1024, // 1MB
	}

	err = admin.CreateTopic(topicConfig)
	if err != nil {
		log.Printf("Failed to create topic: %v", err)
	} else {
		fmt.Println("Topic 'analytics-events' created with custom configuration!")
	}

	// List all topics
	fmt.Println("\nListing all topics...")
	topics, err := admin.ListTopics()
	if err != nil {
		log.Printf("Failed to list topics: %v", err)
	} else {
		fmt.Println("Topics in cluster:")
		for i, topic := range topics {
			fmt.Printf("  %d. %s\n", i+1, topic)
		}
	}

	// Describe a specific topic
	fmt.Println("\nDescribing topic 'user-events'...")
	metadata, err := admin.DescribeTopic("user-events")
	if err != nil {
		log.Printf("Failed to describe topic: %v", err)
	} else {
		fmt.Printf("Topic: %s\n", metadata.Name)
		fmt.Printf("  Partitions: %d\n", len(metadata.Partitions))
		fmt.Printf("  Replication Factor: %d\n", metadata.ReplicationFactor)
		fmt.Printf("  Created At: %s\n", metadata.CreatedAt.Format(time.RFC3339))

		for _, partition := range metadata.Partitions {
			fmt.Printf("  Partition %d:\n", partition.ID)
			fmt.Printf("    Leader: Broker %d\n", partition.Leader)
			fmt.Printf("    Replicas: %v\n", partition.Replicas)
			fmt.Printf("    ISR: %v\n", partition.ISR)
		}
	}

	// Create multiple topics at once
	fmt.Println("\nCreating multiple topics...")
	configs := []streambus.TopicConfig{
		{Name: "orders", Partitions: 12, ReplicationFactor: 3},
		{Name: "payments", Partitions: 12, ReplicationFactor: 3},
		{Name: "shipments", Partitions: 6, ReplicationFactor: 2},
		{Name: "notifications", Partitions: 20, ReplicationFactor: 3},
	}

	err = admin.CreateTopics(configs...)
	if err != nil {
		log.Printf("Failed to create topics: %v", err)
	} else {
		fmt.Println("All topics created successfully!")
	}
}

func clusterInspection() {
	fmt.Println("\n=== Cluster Inspection Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	admin := client.Admin()

	// Get cluster metadata
	ctx := context.Background()
	metadata, err := admin.GetClusterMetadata(ctx)
	if err != nil {
		log.Printf("Failed to get cluster metadata: %v", err)
		return
	}

	// Display cluster information
	fmt.Println("\n📊 Cluster Information:")
	fmt.Printf("  Cluster ID: %s\n", metadata.ClusterID)
	fmt.Printf("  Controller: Broker %d\n", metadata.Controller)
	fmt.Printf("  Total Topics: %d\n", metadata.TotalTopics)
	fmt.Printf("  Total Partitions: %d\n", metadata.TotalPartitions)

	fmt.Println("\n🖥  Brokers:")
	for _, broker := range metadata.Brokers {
		fmt.Printf("  Broker %d:\n", broker.ID)
		fmt.Printf("    Host: %s:%d\n", broker.Host, broker.Port)
		fmt.Printf("    State: %s\n", broker.State)
		fmt.Printf("    Uptime: %s\n", time.Since(broker.StartTime))
		if broker.Rack != "" {
			fmt.Printf("    Rack: %s\n", broker.Rack)
		}
	}

	if len(metadata.Topics) > 0 {
		fmt.Println("\n📑 Topics:")
		for i, topic := range metadata.Topics {
			if i < 10 { // Show first 10 topics
				fmt.Printf("    - %s\n", topic)
			}
		}
		if len(metadata.Topics) > 10 {
			fmt.Printf("    ... and %d more\n", len(metadata.Topics)-10)
		}
	}
}

func consumerGroupManagement() {
	fmt.Println("\n=== Consumer Group Management Example ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	admin := client.Admin()

	// List all consumer groups
	fmt.Println("Listing consumer groups...")
	groups, err := admin.ListConsumerGroups()
	if err != nil {
		log.Printf("Failed to list consumer groups: %v", err)
	} else if len(groups) == 0 {
		fmt.Println("No consumer groups found")
	} else {
		fmt.Println("Consumer Groups:")
		for _, group := range groups {
			fmt.Printf("  - %s\n", group)
		}
	}

	// Describe a specific consumer group
	groupID := "my-consumer-group"
	fmt.Printf("\nDescribing consumer group '%s'...\n", groupID)

	description, err := admin.DescribeConsumerGroup(groupID)
	if err != nil {
		log.Printf("Failed to describe consumer group: %v", err)
	} else {
		fmt.Printf("Group ID: %s\n", description.GroupID)
		fmt.Printf("  State: %s\n", description.State)
		fmt.Printf("  Coordinator: Broker %d\n", description.Coordinator)
		fmt.Printf("  Members: %d\n", len(description.Members))

		for i, member := range description.Members {
			fmt.Printf("\n  Member %d:\n", i+1)
			fmt.Printf("    Member ID: %s\n", member.MemberID)
			fmt.Printf("    Client ID: %s\n", member.ClientID)
			fmt.Printf("    Host: %s\n", member.ClientHost)
			fmt.Printf("    Last Heartbeat: %s ago\n", time.Since(member.HeartbeatTime))

			if len(member.Assignment) > 0 {
				fmt.Println("    Assigned Partitions:")
				for _, assignment := range member.Assignment {
					fmt.Printf("      - %s: %v\n", assignment.Topic, assignment.Partitions)
				}
			}
		}
	}
}

// Advanced admin operations example
func advancedAdminOperations() {
	fmt.Println("\n=== Advanced Admin Operations ===")

	client, _ := streambus.Connect("localhost:9092")
	defer client.Close()

	admin := client.Admin()

	// Alter topic configuration
	fmt.Println("Updating topic configuration...")
	configUpdates := map[string]string{
		"retention.ms":    "604800000", // 7 days
		"segment.ms":      "86400000",  // 1 day
		"compression.type": "gzip",
	}

	err := admin.AlterTopicConfig("user-events", configUpdates)
	if err != nil {
		log.Printf("Failed to update topic config: %v", err)
	} else {
		fmt.Println("Topic configuration updated!")
	}

	// Delete a consumer group (only if idle)
	fmt.Println("\nDeleting idle consumer group...")
	err = admin.DeleteConsumerGroup("old-consumer-group")
	if err != nil {
		log.Printf("Failed to delete consumer group: %v", err)
	} else {
		fmt.Println("Consumer group deleted!")
	}

	// Delete a topic
	fmt.Println("\nDeleting topic...")
	err = admin.DeleteTopic("temp-topic")
	if err != nil {
		log.Printf("Failed to delete topic: %v", err)
	} else {
		fmt.Println("Topic deleted!")
	}
}