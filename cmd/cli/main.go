package main

import (
	"fmt"
	"os"

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

		fmt.Printf("Creating topic '%s' with %d partitions and replication factor %d\n",
			topicName, partitions, replication)

		// TODO: Implement topic creation
		// client := streambus.NewClient(brokers)
		// err := client.CreateTopic(topicName, partitions, replication)

		fmt.Println("Topic created successfully!")
		return nil
	},
}

var topicListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all topics",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing topics...")

		// TODO: Implement topic listing
		// client := streambus.NewClient(brokers)
		// topics, err := client.ListTopics()

		fmt.Println("No topics found")
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

		fmt.Printf("Producing message to topic '%s'\n", topic)
		fmt.Printf("  Key: %s\n", key)
		fmt.Printf("  Message: %s\n", message)

		// TODO: Implement produce
		// producer := streambus.NewProducer(brokers)
		// offset, err := producer.Produce(topic, key, message)

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
		offset, _ := cmd.Flags().GetString("offset")

		fmt.Printf("Consuming from topic '%s'\n", topic)
		fmt.Printf("  Consumer Group: %s\n", group)
		fmt.Printf("  Starting Offset: %s\n", offset)

		// TODO: Implement consume
		// consumer := streambus.NewConsumer(brokers, group)
		// consumer.Subscribe(topic)
		// for msg := range consumer.Messages() {
		//     fmt.Printf("Offset: %d, Key: %s, Value: %s\n", msg.Offset, msg.Key, msg.Value)
		// }

		fmt.Println("Consuming messages... (Press Ctrl+C to stop)")
		return nil
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
		fmt.Println("Cluster Status:")
		fmt.Println()

		// TODO: Implement cluster status
		// client := streambus.NewClient(brokers)
		// status, err := client.ClusterStatus()

		fmt.Println("  Brokers: 0")
		fmt.Println("  Topics: 0")
		fmt.Println("  Partitions: 0")
		fmt.Println("  Status: Unknown")

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

		// TODO: Implement group listing
		// client := streambus.NewClient(brokers)
		// groups, err := client.ListGroups()

		fmt.Println("No consumer groups found")
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
	clusterStatusCmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	clusterCmd.AddCommand(clusterStatusCmd)
	rootCmd.AddCommand(clusterCmd)

	// Consumer group commands
	groupCmd.AddCommand(groupListCmd)
	rootCmd.AddCommand(groupCmd)
}
