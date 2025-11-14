package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gstreamio/streambus/pkg/broker"
	"github.com/gstreamio/streambus/pkg/consensus"
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

	_ = viper.BindPFlag("server.broker_id", rootCmd.PersistentFlags().Lookup("broker-id"))
	_ = viper.BindPFlag("server.host", rootCmd.PersistentFlags().Lookup("host"))
	_ = viper.BindPFlag("server.port", rootCmd.PersistentFlags().Lookup("port"))
	_ = viper.BindPFlag("storage.data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
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

	// Parse configuration from viper
	config, err := parseConfig()
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	fmt.Printf("Broker Configuration:\n")
	fmt.Printf("  Broker ID: %d\n", config.BrokerID)
	fmt.Printf("  Address: %s:%d\n", config.Host, config.Port)
	fmt.Printf("  Data Directory: %s\n", config.DataDir)
	fmt.Printf("  Raft Data Directory: %s\n", config.RaftDataDir)
	fmt.Printf("  HTTP Port: %d\n", config.HTTPPort)
	fmt.Printf("  Raft Peers: %d\n", len(config.RaftPeers))
	fmt.Println()

	// Create broker
	b, err := broker.New(config)
	if err != nil {
		return fmt.Errorf("failed to create broker: %w", err)
	}

	// Start broker
	if err := b.Start(); err != nil {
		return fmt.Errorf("failed to start broker: %w", err)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("StreamBus Broker is running!")
	fmt.Println("========================================")
	fmt.Printf("  Protocol: %s:%d\n", config.Host, config.Port)
	fmt.Printf("  Health:   http://0.0.0.0:%d/health\n", config.HTTPPort)
	fmt.Printf("  Metrics:  http://0.0.0.0:%d/metrics\n", config.HTTPPort)
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop...")

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigCh
	fmt.Printf("\nReceived signal: %v\n", sig)
	fmt.Println("Shutting down gracefully...")

	// Graceful shutdown
	if err := b.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		return err
	}

	fmt.Println("Broker stopped cleanly")
	return nil
}

func parseConfig() (*broker.Config, error) {
	// Get configuration values
	brokerID := viper.GetInt32("server.broker_id")
	if brokerID == 0 {
		return nil, fmt.Errorf("server.broker_id must be set")
	}

	host := viper.GetString("server.host")
	if host == "" {
		host = "0.0.0.0"
	}

	port := viper.GetInt("server.port")
	if port == 0 {
		port = 9092
	}

	grpcPort := viper.GetInt("server.grpc_port")
	if grpcPort == 0 {
		grpcPort = 9093
	}

	httpPort := viper.GetInt("server.http_port")
	if httpPort == 0 {
		httpPort = 8080
	}

	dataDir := viper.GetString("storage.data_dir")
	if dataDir == "" {
		return nil, fmt.Errorf("storage.data_dir must be set")
	}

	raftDataDir := viper.GetString("cluster.raft.data_dir")
	if raftDataDir == "" {
		return nil, fmt.Errorf("cluster.raft.data_dir must be set")
	}

	// Parse Raft peers
	var raftPeers []consensus.Peer
	peersConfig := viper.Get("cluster.peers")
	if peersConfig != nil {
		peersSlice, ok := peersConfig.([]interface{})
		if !ok {
			return nil, fmt.Errorf("cluster.peers must be an array")
		}

		for _, p := range peersSlice {
			peerStr, ok := p.(string)
			if !ok {
				continue
			}

			// Parse peer string: "1:localhost:9093" or "1:broker-1:9093"
			// Split by colon
			parts := strings.Split(peerStr, ":")
			if len(parts) != 3 {
				fmt.Fprintf(os.Stderr, "Warning: Invalid peer format (expected id:host:port): %s\n", peerStr)
				continue
			}

			// Parse ID
			var id uint64
			if _, err := fmt.Sscanf(parts[0], "%d", &id); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Invalid peer ID: %s\n", parts[0])
				continue
			}

			// Parse port
			var port int
			if _, err := fmt.Sscanf(parts[2], "%d", &port); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Invalid peer port: %s\n", parts[2])
				continue
			}

			raftPeers = append(raftPeers, consensus.Peer{
				ID:   id,
				Addr: fmt.Sprintf("%s:%d", parts[1], port),
			})
		}
	}

	if len(raftPeers) == 0 {
		return nil, fmt.Errorf("cluster.peers must have at least one peer")
	}

	logLevel := viper.GetString("observability.logging.level")
	if logLevel == "" {
		logLevel = "info"
	}

	return &broker.Config{
		BrokerID:    brokerID,
		Host:        host,
		Port:        port,
		GRPCPort:    grpcPort,
		HTTPPort:    httpPort,
		DataDir:     dataDir,
		RaftDataDir: raftDataDir,
		RaftPeers:   raftPeers,
		LogLevel:    logLevel,
	}, nil
}
