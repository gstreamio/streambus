package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gstreamio/streambus/ui/backend/services"
)

// ClusterHandler handles cluster-related requests
type ClusterHandler struct {
	brokerService  *services.BrokerService
	metricsService *services.MetricsService
}

// NewClusterHandler creates a new cluster handler
func NewClusterHandler(brokerService *services.BrokerService, metricsService *services.MetricsService) *ClusterHandler {
	return &ClusterHandler{
		brokerService:  brokerService,
		metricsService: metricsService,
	}
}

// GetClusterInfo returns cluster information
func (h *ClusterHandler) GetClusterInfo(c *gin.Context) {
	info, err := h.brokerService.GetClusterInfo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// GetClusterHealth returns cluster health status
func (h *ClusterHandler) GetClusterHealth(c *gin.Context) {
	health, err := h.brokerService.GetClusterHealth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": health,
	})
}

// ListBrokers returns all brokers
func (h *ClusterHandler) ListBrokers(c *gin.Context) {
	brokers, err := h.brokerService.ListBrokers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, brokers)
}

// GetBroker returns a specific broker
func (h *ClusterHandler) GetBroker(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid broker id"})
		return
	}

	broker, err := h.brokerService.GetBroker(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, broker)
}
