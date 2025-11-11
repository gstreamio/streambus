package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/streambus/ui/backend/services"
)

// MetricsHandler handles metrics requests
type MetricsHandler struct {
	metricsService *services.MetricsService
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(metricsService *services.MetricsService) *MetricsHandler {
	return &MetricsHandler{
		metricsService: metricsService,
	}
}

// GetThroughput returns throughput metrics
func (h *MetricsHandler) GetThroughput(c *gin.Context) {
	durationStr := c.DefaultQuery("duration", "1h")
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid duration format"})
		return
	}

	metrics, err := h.metricsService.GetThroughput(c.Request.Context(), duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetLatency returns latency metrics
func (h *MetricsHandler) GetLatency(c *gin.Context) {
	durationStr := c.DefaultQuery("duration", "1h")
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid duration format"})
		return
	}

	metrics, err := h.metricsService.GetLatency(c.Request.Context(), duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetResources returns resource usage metrics
func (h *MetricsHandler) GetResources(c *gin.Context) {
	durationStr := c.DefaultQuery("duration", "1h")
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid duration format"})
		return
	}

	metrics, err := h.metricsService.GetResources(c.Request.Context(), duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}
