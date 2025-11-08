package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/shawntherrien/streambus/pkg/consensus"
	"github.com/shawntherrien/streambus/pkg/metadata"
)

func main() {
	fmt.Println("StreamBus Metadata System Demo")
	fmt.Println("================================")
	fmt.Println()

	// Create temporary directory for Raft data
	dir, err := os.MkdirTemp("", "streambus-demo-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	fmt.Printf("Using data directory: %s\n\n", dir)

	// Step 1: Create a single-node Raft cluster with metadata FSM
	fmt.Println("Step 1: Starting single-node Raft cluster...")
	config := consensus.DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []consensus.Peer{
		{ID: 1, Addr: "localhost:29001"},
	}

	// Create metadata FSM
	fsm := metadata.NewFSM()
	node, err := consensus.NewNode(config, fsm)
	if err != nil {
		panic(err)
	}

	node.SetBindAddr("localhost:29001")
	if err := node.Start(); err != nil {
		panic(err)
	}
	defer node.Stop()

	// Wait for leader election
	fmt.Println("Waiting for leader election...")
	time.Sleep(2 * time.Second)

	if !node.IsLeader() {
		panic("Node failed to become leader")
	}
	fmt.Println("✓ Node elected as leader")

	// Create metadata store with the same FSM that's integrated with Raft
	store := metadata.NewStore(fsm, node)
	ctx := context.Background()

	// Step 2: Register brokers
	fmt.Println("Step 2: Registering brokers...")
	brokers := []metadata.BrokerInfo{
		{
			ID:   1,
			Addr: "localhost:9092",
			Rack: "rack1",
			Resources: metadata.BrokerResources{
				DiskTotal:     1024 * 1024 * 1024 * 100, // 100GB
				DiskAvailable: 1024 * 1024 * 1024 * 80,  // 80GB
				CPUCores:      8,
				MemoryTotal:   16 * 1024 * 1024 * 1024, // 16GB
			},
		},
		{
			ID:   2,
			Addr: "localhost:9093",
			Rack: "rack2",
			Resources: metadata.BrokerResources{
				DiskTotal:     1024 * 1024 * 1024 * 100,
				DiskAvailable: 1024 * 1024 * 1024 * 75,
				CPUCores:      8,
				MemoryTotal:   16 * 1024 * 1024 * 1024,
			},
		},
		{
			ID:   3,
			Addr: "localhost:9094",
			Rack: "rack1",
			Resources: metadata.BrokerResources{
				DiskTotal:     1024 * 1024 * 1024 * 100,
				DiskAvailable: 1024 * 1024 * 1024 * 70,
				CPUCores:      8,
				MemoryTotal:   16 * 1024 * 1024 * 1024,
			},
		},
	}

	for _, broker := range brokers {
		b := broker // Create copy for pointer
		if err := store.RegisterBroker(ctx, &b); err != nil {
			panic(err)
		}
		fmt.Printf("  ✓ Registered broker %d at %s (rack: %s)\n", broker.ID, broker.Addr, broker.Rack)
	}

	// Give time for replication
	time.Sleep(200 * time.Millisecond)

	// Verify brokers
	registeredBrokers := store.ListBrokers()
	fmt.Printf("\n  Total brokers: %d\n", len(registeredBrokers))
	for _, b := range registeredBrokers {
		fmt.Printf("    - Broker %d: %s (Disk: %.1fGB available, CPU: %d cores)\n",
			b.ID, b.Addr, float64(b.Resources.DiskAvailable)/(1024*1024*1024), b.Resources.CPUCores)
	}
	fmt.Println()

	// Step 3: Create topics
	fmt.Println("Step 3: Creating topics...")
	topics := []struct {
		name              string
		numPartitions     int
		replicationFactor int
	}{
		{"user-events", 6, 2},
		{"order-events", 3, 3},
		{"notifications", 12, 2},
	}

	for _, topic := range topics {
		config := metadata.DefaultTopicConfig()
		config.RetentionMs = 7 * 24 * 60 * 60 * 1000 // 7 days
		config.CompressionType = "snappy"

		if err := store.CreateTopic(ctx, topic.name, topic.numPartitions, topic.replicationFactor, config); err != nil {
			panic(err)
		}
		fmt.Printf("  ✓ Created topic '%s' with %d partitions (RF=%d)\n",
			topic.name, topic.numPartitions, topic.replicationFactor)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify topics
	createdTopics := store.ListTopics()
	fmt.Printf("\n  Total topics: %d\n", len(createdTopics))
	for _, t := range createdTopics {
		fmt.Printf("    - %s: %d partitions, RF=%d, retention=%dh\n",
			t.Name, t.NumPartitions, t.ReplicationFactor, t.Config.RetentionMs/(60*60*1000))
	}
	fmt.Println()

	// Step 4: Allocate and create partitions
	fmt.Println("Step 4: Allocating partitions to brokers...")
	totalPartitions := 0

	for _, topic := range topics {
		// Allocate partitions using round-robin
		partitions, err := store.AllocatePartitions(topic.name, topic.numPartitions, topic.replicationFactor)
		if err != nil {
			panic(err)
		}

		// Create each partition
		for _, partition := range partitions {
			if err := store.CreatePartition(ctx, partition); err != nil {
				panic(err)
			}
		}

		totalPartitions += len(partitions)
		fmt.Printf("  ✓ Allocated %d partitions for topic '%s'\n", len(partitions), topic.name)
	}

	time.Sleep(200 * time.Millisecond)
	fmt.Printf("\n  Total partitions created: %d\n\n", totalPartitions)

	// Step 5: Show partition assignments
	fmt.Println("Step 5: Partition assignments...")
	for _, topic := range topics {
		fmt.Printf("\n  Topic: %s\n", topic.name)
		partitions := store.ListPartitions(topic.name)
		for _, p := range partitions {
			fmt.Printf("    Partition %d: Leader=%d, Replicas=%v, ISR=%v\n",
				p.Partition, p.Leader, p.Replicas, p.ISR)
		}
	}
	fmt.Println()

	// Step 6: Simulate leader election for a partition
	fmt.Println("Step 6: Simulating leader failover...")
	fmt.Println("  Scenario: Broker 1 fails, promoting broker 2 as leader for user-events:0")

	oldPartition, _ := store.GetPartition("user-events", 0)
	fmt.Printf("  Before: Leader=%d, Epoch=%d, ISR=%v\n",
		oldPartition.Leader, oldPartition.LeaderEpoch, oldPartition.ISR)

	// Update leader to broker 2 with new epoch
	if err := store.UpdateLeader(ctx, "user-events", 0, 2, oldPartition.LeaderEpoch+1); err != nil {
		panic(err)
	}

	time.Sleep(200 * time.Millisecond)

	newPartition, _ := store.GetPartition("user-events", 0)
	fmt.Printf("  After:  Leader=%d, Epoch=%d, ISR=%v\n",
		newPartition.Leader, newPartition.LeaderEpoch, newPartition.ISR)
	fmt.Println("  ✓ Leader election completed")

	// Step 7: Simulate ISR shrink (replica falling behind)
	fmt.Println("Step 7: Simulating ISR shrink...")
	fmt.Println("  Scenario: Replica 3 falls behind on order-events:0")

	oldPartition2, _ := store.GetPartition("order-events", 0)
	fmt.Printf("  Before: ISR=%v\n", oldPartition2.ISR)

	// Remove broker 3 from ISR
	newISR := []uint64{1, 2}
	if err := store.UpdateISR(ctx, "order-events", 0, newISR); err != nil {
		panic(err)
	}

	time.Sleep(200 * time.Millisecond)

	newPartition2, _ := store.GetPartition("order-events", 0)
	fmt.Printf("  After:  ISR=%v\n", newPartition2.ISR)
	fmt.Println("  ✓ ISR updated")

	// Step 8: Update broker status
	fmt.Println("Step 8: Updating broker status...")
	fmt.Println("  Marking broker 1 as draining for maintenance")

	if err := store.UpdateBroker(ctx, 1, metadata.BrokerStatusDraining, brokers[0].Resources); err != nil {
		panic(err)
	}

	time.Sleep(200 * time.Millisecond)

	broker1, _ := store.GetBroker(1)
	fmt.Printf("  Broker 1 status: %s\n", broker1.Status)
	fmt.Println("  ✓ Broker status updated")

	// Step 9: Show final cluster state
	fmt.Println("Step 9: Final cluster state...")
	state := store.GetState()
	fmt.Printf("\n  Cluster Version: %d\n", state.Version)
	fmt.Printf("  Last Modified: %s\n", state.LastModified.Format(time.RFC3339))
	fmt.Printf("\n  Summary:")
	fmt.Printf("    - Brokers: %d\n", len(state.Brokers))
	fmt.Printf("    - Topics: %d\n", len(state.Topics))
	fmt.Printf("    - Partitions: %d\n", len(state.Partitions))

	// Calculate some statistics
	var totalReplicas int
	leadersPerBroker := make(map[uint64]int)
	for _, p := range state.Partitions {
		totalReplicas += len(p.Replicas)
		leadersPerBroker[p.Leader]++
	}

	fmt.Printf("\n  Partition Statistics:")
	fmt.Printf("    - Total partition replicas: %d\n", totalReplicas)
	fmt.Printf("    - Average replicas per partition: %.1f\n",
		float64(totalReplicas)/float64(len(state.Partitions)))
	fmt.Printf("    - Leaders per broker:")
	for brokerID := uint64(1); brokerID <= 3; brokerID++ {
		count := leadersPerBroker[brokerID]
		fmt.Printf("        Broker %d: %d partitions (%.1f%%)\n",
			brokerID, count, float64(count)/float64(len(state.Partitions))*100)
	}

	fmt.Println("\n✓ Demo completed successfully!")
	fmt.Println("\nKey Takeaways:")
	fmt.Println("  1. Metadata is replicated via Raft consensus")
	fmt.Println("  2. All operations are atomic and consistent")
	fmt.Println("  3. Leader election and ISR management work correctly")
	fmt.Println("  4. Partition allocation balances load across brokers")
	fmt.Println("  5. Broker status tracking enables maintenance operations")
}
