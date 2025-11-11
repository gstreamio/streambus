package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shawntherrien/streambus/ui/backend/services"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	brokerService  *services.BrokerService
	metricsService *services.MetricsService
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(brokerService *services.BrokerService, metricsService *services.MetricsService) *WebSocketHandler {
	return &WebSocketHandler{
		brokerService:  brokerService,
		metricsService: metricsService,
	}
}

// Message represents a WebSocket message
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// HandleWebSocket handles WebSocket connections
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Println("WebSocket client connected")

	// Send initial data
	h.sendClusterInfo(conn)

	// Start sending updates
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ctx := context.Background()

	for {
		select {
		case <-ticker.C:
			// Send periodic updates
			if err := h.sendClusterInfo(conn); err != nil {
				log.Printf("WebSocket send error: %v", err)
				return
			}

			if err := h.sendMetrics(ctx, conn); err != nil {
				log.Printf("WebSocket send error: %v", err)
				return
			}

		default:
			// Read messages from client (for heartbeat)
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}
		}
	}
}

func (h *WebSocketHandler) sendClusterInfo(conn *websocket.Conn) error {
	ctx := context.Background()
	info, err := h.brokerService.GetClusterInfo(ctx)
	if err != nil {
		return err
	}

	msg := Message{
		Type: "cluster_info",
		Data: info,
	}

	return conn.WriteJSON(msg)
}

func (h *WebSocketHandler) sendMetrics(ctx context.Context, conn *websocket.Conn) error {
	throughput, err := h.metricsService.GetThroughput(ctx, 5*time.Minute)
	if err != nil {
		return err
	}

	msg := Message{
		Type: "metrics",
		Data: throughput,
	}

	return conn.WriteJSON(msg)
}
