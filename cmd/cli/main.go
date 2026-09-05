package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "streambus-cli",
	Short: "StreamBus command-line interface",
	Long: `StreamBus CLI - Command-line tool for managing StreamBus clusters.

Use this tool to create topics, produce/consume messages, manage consumer
groups, and administer your StreamBus cluster.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, buildTime),
}

var topicCmd = &cobra.Command{
	Use:   "topic",
	Short: "Manage topics",
	Long:  "Create, delete, describe, and list topics",
}

var topicCreateCmd = &cobra.Command{
	Use:   "create <topic-name>",
	Short: "Create a new topic",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		topicName := args[0]
		partitions, _ := cmd.Flags().GetInt("partitions")
		replication, _ := cmd.Flags().GetInt("replication-factor")

		brokers, _ := cmd.Flags().GetStringSlice("brokers")

		fmt.Printf("Creating topic '%s' with %d partitions and replication factor %d\n",
			topicName, partitions, replication)

		// Create client
		config := client.DefaultConfig()
		config.Brokers = brokers
		c, err := client.New(config)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		// Create topic (using administrative API)
		if err := c.CreateTopic(cmd.Context(), topicName, uint32(partitions), uint16(replication)); err != nil {
			return fmt.Errorf("failed to create topic: %w", err)
		}

		fmt.Println("Topic created successfully!")
		return nil
	},
}

var topicListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all topics",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing topics...")

		brokers, _ := cmd.Flags().GetStringSlice("brokers")

		// Create client
		config := client.DefaultConfig()
		config.Brokers = brokers
		c, err := client.New(config)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		// List topics
		topics, err := c.ListTopics(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to list topics: %w", err)
		}

		if len(topics) == 0 {
			fmt.Println("No topics found")
			return nil
		}

		fmt.Printf("\nFound %d topic(s):\n", len(topics))
		for _, topicName := range topics {
			fmt.Printf("  - %s\n", topicName)
		}

		return nil
	},
}

var produceCmd = &cobra.Command{
	Use:   "produce <topic>",
	Short: "Produce messages to a topic",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		topic := args[0]
		message, _ := cmd.Flags().GetString("message")
		key, _ := cmd.Flags().GetString("key")
		brokers, _ := cmd.Flags().GetStringSlice("brokers")

		// Read from stdin if no message provided
		if message == "" {
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				message = scanner.Text()
			}
		}

		fmt.Printf("Producing message to topic '%s'\n", topic)
		fmt.Printf("  Key: %s\n", key)
		fmt.Printf("  Message: %s\n", message)

		// Create client
		config := client.DefaultConfig()
		config.Brokers = brokers
		c, err := client.New(config)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		// Create producer
		producer := client.NewProducer(c)
		defer producer.Close()

		// Send message
		err = producer.Send(cmd.Context(), topic, []byte(key), []byte(message))
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}

		// Flush
		if err := producer.Flush(cmd.Context(), topic); err != nil {
			return fmt.Errorf("failed to flush: %w", err)
		}

		fmt.Println("Message produced successfully!")
		return nil
	},
}

var consumeCmd = &cobra.Command{
	Use:   "consume <topic>",
	Short: "Consume messages from a topic",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		topic := args[0]
		group, _ := cmd.Flags().GetString("group")
		offsetStr, _ := cmd.Flags().GetString("offset")
		maxMessages, _ := cmd.Flags().GetInt("max-messages")
		brokers, _ := cmd.Flags().GetStringSlice("brokers")

		// Parse offset flag
		var startOffset int64
		switch offsetStr {
		case "earliest":
			startOffset = 0
		case "latest":
			startOffset = -1
		default:
			// Try parsing as number
			parsed, err := fmt.Sscanf(offsetStr, "%d", &startOffset)
			if err != nil || parsed != 1 {
				return fmt.Errorf("invalid offset value: %s (use 'earliest', 'latest', or a number)", offsetStr)
			}
		}

		fmt.Printf("Consuming from topic '%s'\n", topic)
		fmt.Printf("  Consumer Group: %s\n", group)
		fmt.Printf("  Starting Offset: %s (resolved to %d)\n", offsetStr, startOffset)

		// Create client
		config := client.DefaultConfig()
		config.Brokers = brokers

		// Configure consumer with parsed offset
		config.ConsumerConfig.StartOffset = startOffset

		c, err := client.New(config)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		// Create consumer with configured offset
		consumer := client.NewConsumer(c, topic, 0)
		defer consumer.Close()

		fmt.Println("Consuming messages... (Press Ctrl+C to stop)")

		// Set up signal handling. Cancelling ctx makes Poll return
		// immediately instead of waiting for its next tick.
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		go func() {
			<-sigChan
			cancel()
		}()

		messagesConsumed := 0

		// Poll for messages
		for {
			if ctx.Err() != nil {
				fmt.Printf("\n\nConsumed %d messages\n", messagesConsumed)
				return nil
			}

			err := consumer.Poll(ctx, 1*time.Second, func(messages []protocol.Message) error {
				for _, msg := range messages {
					fmt.Printf("Offset: %d, Key: %s, Value: %s\n",
						msg.Offset, string(msg.Key), string(msg.Value))
					messagesConsumed++

					if maxMessages > 0 && messagesConsumed >= maxMessages {
						return fmt.Errorf("reached max messages")
					}
				}
				return nil
			})

			if err != nil {
				if err.Error() == "reached max messages" {
					fmt.Printf("\nConsumed %d messages\n", messagesConsumed)
					return nil
				}
				if ctx.Err() != nil {
					fmt.Printf("\n\nConsumed %d messages\n", messagesConsumed)
					return nil
				}
				// Ignore timeout errors
				continue
			}
		}
	},
}

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage cluster",
	Long:  "View and manage cluster information",
}

var clusterStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster status",
	RunE: func(cmd *cobra.Command, args []string) error {
		adminAddr, _ := cmd.Flags().GetString("admin-addr")
		verbose, _ := cmd.Flags().GetBool("verbose")

		admin := newAdminClient(adminAddr)

		// Fetch before printing anything: a broker that can't be reached
		// must fail the command outright, never fall back to a header
		// followed by plausible-looking zeros.
		info, err := admin.ClusterInfo(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to get cluster status: %w", err)
		}

		fmt.Println("Cluster Status:")
		fmt.Println()
		fmt.Printf("  Cluster ID:  %s\n", info.ClusterID)
		fmt.Printf("  Controller:  %d\n", info.ControllerID)
		fmt.Printf("  Version:     %s\n", info.Version)
		fmt.Printf("  Brokers:     %d active / %d total\n", info.ActiveBrokers, info.TotalBrokers)
		fmt.Printf("  Topics:      %d\n", info.TotalTopics)
		fmt.Printf("  Partitions:  %d\n", info.TotalPartitions)
		fmt.Printf("  Uptime:      %s\n", info.Uptime)

		if !verbose {
			return nil
		}

		brokers, err := admin.Brokers(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to get broker list: %w", err)
		}

		fmt.Printf("\nBrokers (%d):\n", len(brokers))
		for _, b := range brokers {
			leader := ""
			if b.Leader {
				leader = " (leader)"
			}
			fmt.Printf("  - [%d] %s:%d  status=%s  version=%s  uptime=%s%s\n",
				b.ID, b.Host, b.Port, b.Status, b.Version, b.Uptime, leader)
		}

		return nil
	},
}

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage consumer groups",
	Long:  "List, describe, and delete consumer groups",
}

var groupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List consumer groups",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing consumer groups...")

		adminAddr, _ := cmd.Flags().GetString("admin-addr")
		admin := newAdminClient(adminAddr)

		groups, err := admin.ConsumerGroups(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to list consumer groups: %w", err)
		}

		if len(groups) == 0 {
			fmt.Println("No consumer groups found")
			return nil
		}

		fmt.Printf("\nFound %d consumer group(s):\n", len(groups))
		for _, g := range groups {
			fmt.Printf("  - %s (state: %s, protocol: %s, members: %d)\n",
				g.GroupID, g.State, g.Protocol, len(g.Members))
		}

		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringSliceP("brokers", "b", []string{"localhost:9092"},
		"Comma-separated list of broker addresses")

	// Topic commands
	topicCreateCmd.Flags().IntP("partitions", "p", 10, "Number of partitions")
	topicCreateCmd.Flags().IntP("replication-factor", "r", 3, "Replication factor")
	topicCmd.AddCommand(topicCreateCmd)
	topicCmd.AddCommand(topicListCmd)
	rootCmd.AddCommand(topicCmd)

	// Produce command
	produceCmd.Flags().StringP("message", "m", "", "Message to produce (or use stdin)")
	produceCmd.Flags().StringP("key", "k", "", "Message key")
	produceCmd.Flags().StringP("headers", "H", "", "Message headers (key:value)")
	rootCmd.AddCommand(produceCmd)

	// Consume command
	consumeCmd.Flags().StringP("group", "g", "", "Consumer group ID")
	consumeCmd.Flags().StringP("offset", "o", "latest", "Starting offset (earliest, latest, or numeric)")
	consumeCmd.Flags().IntP("max-messages", "n", -1, "Maximum messages to consume (-1 for unlimited)")
	rootCmd.AddCommand(consumeCmd)

	// Cluster commands
	clusterStatusCmd.Flags().BoolP("verbose", "v", false, "Also list individual brokers")
	clusterCmd.PersistentFlags().String("admin-addr", "localhost:8080", "Admin HTTP API address (host:port)")
	clusterCmd.AddCommand(clusterStatusCmd)
	rootCmd.AddCommand(clusterCmd)

	// Consumer group commands
	groupCmd.PersistentFlags().String("admin-addr", "localhost:8080", "Admin HTTP API address (host:port)")
	groupCmd.AddCommand(groupListCmd)
	rootCmd.AddCommand(groupCmd)
}
