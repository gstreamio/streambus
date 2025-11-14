package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gstreamio/streambus/ui/backend/handlers"
	"github.com/gstreamio/streambus/ui/backend/middleware"
	"github.com/gstreamio/streambus/ui/backend/services"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type Config struct {
	Port              int
	Brokers           []string
	PrometheusURL     string
	JWTSecret         string
	EnableAuth        bool
	StaticDir         string
	CORSOrigins       []string
	WebSocketEnabled  bool
}

func main() {
	config := parseFlags()

	// Initialize services
	brokerService := services.NewBrokerService(config.Brokers)
	topicService := services.NewTopicService(brokerService)
	consumerGroupService := services.NewConsumerGroupService(brokerService)
	metricsService := services.NewMetricsService(config.PrometheusURL)

	// Setup router
	router := setupRouter(config, brokerService, topicService, consumerGroupService, metricsService)

	// Create HTTP server
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("StreamBus UI starting on port %d", config.Port)
		log.Printf("Version: %s, Commit: %s, Build Time: %s", version, commit, buildTime)
		log.Printf("Connected to brokers: %v", config.Brokers)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func parseFlags() *Config {
	config := &Config{}

	flag.IntVar(&config.Port, "port", 8080, "Server port")
	brokersStr := flag.String("brokers", "localhost:9092", "Comma-separated list of broker addresses")
	flag.StringVar(&config.PrometheusURL, "prometheus-url", "http://localhost:9090", "Prometheus URL")
	flag.StringVar(&config.JWTSecret, "jwt-secret", "streambus-secret", "JWT secret key")
	flag.BoolVar(&config.EnableAuth, "enable-auth", false, "Enable authentication")
	flag.StringVar(&config.StaticDir, "static-dir", "./static", "Static files directory")
	corsOriginsStr := flag.String("cors-origins", "http://localhost:3000", "Comma-separated CORS origins")
	flag.BoolVar(&config.WebSocketEnabled, "websocket", true, "Enable WebSocket")

	flag.Parse()

	// Parse brokers from environment or flag
	if brokersEnv := os.Getenv("STREAMBUS_BROKERS"); brokersEnv != "" {
		*brokersStr = brokersEnv
	}
	config.Brokers = strings.Split(*brokersStr, ",")

	// Parse CORS origins
	config.CORSOrigins = strings.Split(*corsOriginsStr, ",")

	// Override from environment
	if portEnv := os.Getenv("STREAMBUS_PORT"); portEnv != "" {
		fmt.Sscanf(portEnv, "%d", &config.Port)
	}
	if prometheusURL := os.Getenv("STREAMBUS_PROMETHEUS_URL"); prometheusURL != "" {
		config.PrometheusURL = prometheusURL
	}
	if jwtSecret := os.Getenv("STREAMBUS_JWT_SECRET"); jwtSecret != "" {
		config.JWTSecret = jwtSecret
	}

	return config
}

func setupRouter(
	config *Config,
	brokerService *services.BrokerService,
	topicService *services.TopicService,
	consumerGroupService *services.ConsumerGroupService,
	metricsService *services.MetricsService,
) *gin.Engine {
	// Set Gin mode
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS
	corsConfig := cors.Config{
		AllowOrigins:     config.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	// Initialize handlers
	clusterHandler := handlers.NewClusterHandler(brokerService, metricsService)
	topicHandler := handlers.NewTopicHandler(topicService)
	consumerGroupHandler := handlers.NewConsumerGroupHandler(consumerGroupService)
	metricsHandler := handlers.NewMetricsHandler(metricsService)

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": version,
			"commit":  commit,
		})
	})

	// API routes
	api := router.Group("/api")
	{
		// Cluster
		api.GET("/cluster", clusterHandler.GetClusterInfo)
		api.GET("/cluster/health", clusterHandler.GetClusterHealth)
		api.GET("/cluster/brokers", clusterHandler.ListBrokers)
		api.GET("/cluster/brokers/:id", clusterHandler.GetBroker)

		// Topics
		api.GET("/topics", topicHandler.ListTopics)
		api.POST("/topics", topicHandler.CreateTopic)
		api.GET("/topics/:name", topicHandler.GetTopic)
		api.DELETE("/topics/:name", topicHandler.DeleteTopic)
		api.PUT("/topics/:name/config", topicHandler.UpdateTopicConfig)
		api.GET("/topics/:name/partitions", topicHandler.GetPartitions)
		api.GET("/topics/:name/messages", topicHandler.GetMessages)

		// Consumer Groups
		api.GET("/consumer-groups", consumerGroupHandler.ListGroups)
		api.GET("/consumer-groups/:id", consumerGroupHandler.GetGroup)
		api.GET("/consumer-groups/:id/lag", consumerGroupHandler.GetGroupLag)
		api.POST("/consumer-groups/:id/reset-offset", consumerGroupHandler.ResetOffset)

		// Metrics
		api.GET("/metrics/throughput", metricsHandler.GetThroughput)
		api.GET("/metrics/latency", metricsHandler.GetLatency)
		api.GET("/metrics/resources", metricsHandler.GetResources)
	}

	// WebSocket for real-time updates
	if config.WebSocketEnabled {
		wsHandler := handlers.NewWebSocketHandler(brokerService, metricsService)
		router.GET("/ws", wsHandler.HandleWebSocket)
	}

	// Serve static files (frontend)
	if _, err := os.Stat(config.StaticDir); err == nil {
		router.Static("/assets", config.StaticDir+"/assets")
		router.StaticFile("/", config.StaticDir+"/index.html")
		router.NoRoute(func(c *gin.Context) {
			c.File(config.StaticDir + "/index.html")
		})
	}

	return router
}
