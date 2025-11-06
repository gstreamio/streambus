package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"

	cfgFile string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "streambus-broker",
	Short: "StreamBus distributed streaming platform broker",
	Long: `StreamBus Broker - A high-performance, distributed streaming platform.

Built from the ground up in Go, StreamBus delivers ultra-low latency and
high throughput for your event streaming needs.`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, buildTime),
	RunE:    run,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config/broker.yaml)")
	rootCmd.PersistentFlags().String("broker-id", "", "broker ID (overrides config file)")
	rootCmd.PersistentFlags().String("host", "", "host to bind (overrides config file)")
	rootCmd.PersistentFlags().Int("port", 0, "port to bind (overrides config file)")
	rootCmd.PersistentFlags().String("data-dir", "", "data directory (overrides config file)")

	viper.BindPFlag("server.broker_id", rootCmd.PersistentFlags().Lookup("broker-id"))
	viper.BindPFlag("server.host", rootCmd.PersistentFlags().Lookup("host"))
	viper.BindPFlag("server.port", rootCmd.PersistentFlags().Lookup("port"))
	viper.BindPFlag("storage.data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath("./config")
		viper.AddConfigPath("/etc/streambus")
		viper.SetConfigName("broker")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("STREAMBUS")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Fprintf(os.Stderr, "Warning: Config file not found, using defaults\n")
		} else {
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stdout, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func run(cmd *cobra.Command, args []string) error {
	fmt.Printf("Starting StreamBus Broker %s\n", version)
	fmt.Printf("Commit: %s, Built: %s\n", commit, buildTime)
	fmt.Println()

	// TODO: Initialize broker components
	// - Storage engine
	// - Network server
	// - Replication engine
	// - Consensus (Raft)
	// - Metrics/monitoring

	// Get configuration
	brokerID := viper.GetInt("server.broker_id")
	host := viper.GetString("server.host")
	port := viper.GetInt("server.port")
	dataDir := viper.GetString("storage.data_dir")

	fmt.Printf("Broker Configuration:\n")
	fmt.Printf("  Broker ID: %d\n", brokerID)
	fmt.Printf("  Address: %s:%d\n", host, port)
	fmt.Printf("  Data Directory: %s\n", dataDir)
	fmt.Println()

	// TODO: Validate configuration
	if brokerID == 0 {
		return fmt.Errorf("broker_id must be set")
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx // TODO: Use context when broker is implemented

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Start broker
	// broker, err := broker.New(config)
	// if err != nil {
	//     return fmt.Errorf("failed to create broker: %w", err)
	// }
	//
	// if err := broker.Start(ctx); err != nil {
	//     return fmt.Errorf("failed to start broker: %w", err)
	// }

	fmt.Println("Broker started successfully!")
	fmt.Println("Press Ctrl+C to stop...")

	// Wait for shutdown signal
	sig := <-sigCh
	fmt.Printf("\nReceived signal: %v\n", sig)
	fmt.Println("Shutting down gracefully...")

	// TODO: Graceful shutdown
	// broker.Shutdown(ctx)

	fmt.Println("Broker stopped")
	return nil
}
