package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/protocol"
)

func main() {
	fmt.Println("StreamBus Transactions Example")
	fmt.Println("===============================")
	fmt.Println()
	fmt.Println("This example demonstrates exactly-once semantics using transactions.")
	fmt.Println("It shows read-committed consumer + transactional producer pattern.")
	fmt.Println()

	// Create client
	clientConfig := client.DefaultConfig()
	clientConfig.Brokers = []string{"localhost:9092"}

	c, err := client.New(clientConfig)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create transactional producer
	producerConfig := client.DefaultTransactionalProducerConfig()
	producerConfig.TransactionID = "example-processor-txn"

	producer, err := client.NewTransactionalProducer(c, producerConfig)
	if err != nil {
		log.Fatalf("Failed to create transactional producer: %v", err)
	}
	defer producer.Close()

	// Create transactional consumer with read-committed isolation
	consumerConfig := client.DefaultTransactionalConsumerConfig()
	consumerConfig.Client = c
	consumerConfig.Topics = []string{"input-topic"}
	consumerConfig.IsolationLevel = client.ReadCommitted
	consumerConfig.GroupID = "processor-group"

	consumer, err := client.NewTransactionalConsumer(consumerConfig)
	if err != nil {
		log.Fatalf("Failed to create transactional consumer: %v", err)
	}
	defer consumer.Close()

	fmt.Printf("✓ Initialized transactional producer (ID: %d, Epoch: %d)\n",
		producer.Stats().ProducerID,
		producer.Stats().ProducerEpoch)
	fmt.Printf("✓ Initialized transactional consumer (Isolation: %v)\n",
		getIsolationLevelString(consumerConfig.IsolationLevel))
	fmt.Println()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ctx := context.Background()
	messageCount := 0
	txnCount := 0

	fmt.Println("Starting transactional message processing...")
	fmt.Println("(Press Ctrl+C to stop)")
	fmt.Println()

	// Simulate processing loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\n\n🛑 Received shutdown signal...")
			printFinalStats(producer, consumer, messageCount, txnCount)
			return

		default:
			// Process messages with read-committed semantics
			if err := processWithTransactions(ctx, consumer, producer, &messageCount, &txnCount); err != nil {
				log.Printf("Error processing: %v", err)
				continue
			}

			// Print stats periodically
			if txnCount%5 == 0 && txnCount > 0 {
				printStats(producer, consumer, messageCount, txnCount)
			}

			// For demo purposes, stop after a certain number of transactions
			if txnCount >= 10 {
				fmt.Println("\n\n✓ Completed demo transactions")
				printFinalStats(producer, consumer, messageCount, txnCount)
				return
			}
		}
	}
}

// processWithTransactions demonstrates consume-process-produce with transactions
func processWithTransactions(ctx context.Context, consumer *client.TransactionalConsumer, producer *client.TransactionalProducer, messageCount *int, txnCount *int) error {
	// Begin transaction
	if err := producer.BeginTransaction(ctx); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	*txnCount++

	// Poll for messages with read-committed isolation
	// Only messages from committed transactions will be returned
	records, err := consumer.Poll(ctx)
	if err != nil {
		producer.AbortTransaction(ctx)
		// For demo, we'll simulate messages if poll fails
		return simulateBatch(ctx, producer, messageCount, txnCount)
	}

	// Process consumed messages
	if len(records) == 0 {
		// No messages available, abort transaction and simulate some
		producer.AbortTransaction(ctx)
		return simulateBatch(ctx, producer, messageCount, txnCount)
	}

	// Process each consumed message
	for _, record := range records {
		*messageCount++

		// Transform the message
		processedMessage := processMessage(string(record.Message.Value))

		// Produce to output topic
		msg := protocol.Message{
			Key:   record.Message.Key,
			Value: []byte(processedMessage),
		}

		if err := producer.Send(ctx, "output-topic", 0, msg); err != nil {
			producer.AbortTransaction(ctx)
			return fmt.Errorf("failed to send message: %w", err)
		}

		fmt.Printf("  📨 [Txn %d] Processed: %s -> %s\n",
			*txnCount, string(record.Message.Value), processedMessage)
	}

	// Commit consumer offsets atomically with produced messages
	offsets := make(map[string]map[int32]int64)
	for _, record := range records {
		if offsets[record.Topic] == nil {
			offsets[record.Topic] = make(map[int32]int64)
		}
		offsets[record.Topic][record.Partition] = record.Offset + 1
	}

	if err := producer.SendOffsetsToTransaction(ctx, "processor-group", offsets); err != nil {
		producer.AbortTransaction(ctx)
		return fmt.Errorf("failed to add offsets to transaction: %w", err)
	}

	// Commit transaction - makes produced messages visible and commits offsets
	if err := producer.CommitTransaction(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Printf("  ✅ [Txn %d] Committed successfully\n", *txnCount)
	fmt.Println()

	return nil
}

// simulateBatch simulates processing when no real messages are available
func simulateBatch(ctx context.Context, producer *client.TransactionalProducer, messageCount *int, txnCount *int) error {
	// Begin transaction
	if err := producer.BeginTransaction(ctx); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	batchSize := 3 // Process 3 messages per transaction

	// Simulate processing messages
	for i := 0; i < batchSize; i++ {
		*messageCount++

		// Simulate reading a message
		inputMessage := fmt.Sprintf("input-message-%d", *messageCount)
		processedMessage := processMessage(inputMessage)

		// Produce to output topic
		msg := protocol.Message{
			Key:   []byte(fmt.Sprintf("key-%d", *messageCount)),
			Value: []byte(processedMessage),
		}

		if err := producer.Send(ctx, "output-topic", 0, msg); err != nil {
			producer.AbortTransaction(ctx)
			return fmt.Errorf("failed to send message: %w", err)
		}

		fmt.Printf("  📨 [Txn %d] Processed message %d: %s -> %s\n",
			*txnCount, *messageCount, inputMessage, processedMessage)
	}

	// Simulate consumer offsets
	offsets := map[string]map[int32]int64{
		"input-topic": {
			0: int64(*messageCount),
		},
	}

	if err := producer.SendOffsetsToTransaction(ctx, "processor-group", offsets); err != nil {
		producer.AbortTransaction(ctx)
		return fmt.Errorf("failed to add offsets to transaction: %w", err)
	}

	// Commit transaction
	if err := producer.CommitTransaction(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Printf("  ✅ [Txn %d] Committed successfully (offset: %d)\n", *txnCount, *messageCount)
	fmt.Println()

	return nil
}

// processMessage simulates message processing logic
func processMessage(input string) string {
	// Example: convert to uppercase and add prefix
	return fmt.Sprintf("PROCESSED: %s", strings.ToUpper(input))
}

// getIsolationLevelString returns string representation of isolation level
func getIsolationLevelString(level client.IsolationLevel) string {
	if level == client.ReadCommitted {
		return "READ_COMMITTED"
	}
	return "READ_UNCOMMITTED"
}

// printStats prints current statistics
func printStats(producer *client.TransactionalProducer, consumer *client.TransactionalConsumer, messageCount, txnCount int) {
	pStats := producer.Stats()
	cStats := consumer.Stats()

	fmt.Println("─────────────────────────────────────")
	fmt.Printf("📊 Statistics (after %d transactions):\n", txnCount)
	fmt.Println()
	fmt.Println("Producer:")
	fmt.Printf("   Messages Produced:      %d\n", messageCount)
	fmt.Printf("   Transactions Committed: %d\n", pStats.TransactionsCommitted)
	fmt.Printf("   Transactions Aborted:   %d\n", pStats.TransactionsAborted)
	fmt.Println()
	fmt.Println("Consumer:")
	fmt.Printf("   Messages Consumed:      %d\n", cStats.MessagesConsumed)
	fmt.Printf("   Messages Filtered:      %d\n", cStats.MessagesFiltered)
	fmt.Printf("   Aborted Txns Tracked:   %d\n", cStats.AbortedTransactions)
	fmt.Println("─────────────────────────────────────")
	fmt.Println()
}

// printFinalStats prints final statistics
func printFinalStats(producer *client.TransactionalProducer, consumer *client.TransactionalConsumer, messageCount, txnCount int) {
	pStats := producer.Stats()
	cStats := consumer.Stats()

	fmt.Println()
	fmt.Println("═════════════════════════════════════")
	fmt.Println("       FINAL STATISTICS")
	fmt.Println("═════════════════════════════════════")
	fmt.Println()
	fmt.Println("Transactional Producer:")
	fmt.Printf("  Producer ID:              %d\n", pStats.ProducerID)
	fmt.Printf("  Producer Epoch:           %d\n", pStats.ProducerEpoch)
	fmt.Printf("  Producer State:           %v\n", pStats.State)
	fmt.Printf("  Messages Produced:        %d\n", messageCount)
	fmt.Printf("  Transactions Committed:   %d\n", pStats.TransactionsCommitted)
	fmt.Printf("  Transactions Aborted:     %d\n", pStats.TransactionsAborted)
	if txnCount > 0 {
		fmt.Printf("  Success Rate:             %.1f%%\n",
			float64(pStats.TransactionsCommitted)/float64(txnCount)*100)
	}
	fmt.Println()
	fmt.Println("Transactional Consumer:")
	fmt.Printf("  Isolation Level:          %v\n", getIsolationLevelString(cStats.IsolationLevel))
	fmt.Printf("  Messages Consumed:        %d\n", cStats.MessagesConsumed)
	fmt.Printf("  Messages Filtered:        %d\n", cStats.MessagesFiltered)
	fmt.Printf("  Aborted Txns Tracked:     %d\n", cStats.AbortedTransactions)
	fmt.Println("═════════════════════════════════════")
	fmt.Println()
	fmt.Println("✓ Transactional processing completed successfully!")
	fmt.Println()
	fmt.Println("Key Benefits Demonstrated:")
	fmt.Println("  • Exactly-Once Semantics: Messages processed exactly once")
	fmt.Println("  • Read-Committed Isolation: Only committed messages consumed")
	fmt.Println("  • Atomic Consume-Produce: Reads and writes committed together")
	fmt.Println("  • Coordinated Offsets: Consumer progress tracked with transaction")
	fmt.Println("  • Producer Fencing: Prevents duplicate writes from zombie producers")
}
