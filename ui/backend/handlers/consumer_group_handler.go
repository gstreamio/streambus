package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/streambus/ui/backend/services"
)

// ConsumerGroupHandler handles consumer group requests
type ConsumerGroupHandler struct {
	consumerGroupService *services.ConsumerGroupService
}

// NewConsumerGroupHandler creates a new consumer group handler
func NewConsumerGroupHandler(consumerGroupService *services.ConsumerGroupService) *ConsumerGroupHandler {
	return &ConsumerGroupHandler{
		consumerGroupService: consumerGroupService,
	}
}

// ListGroups returns all consumer groups
func (h *ConsumerGroupHandler) ListGroups(c *gin.Context) {
	groups, err := h.consumerGroupService.ListGroups(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, groups)
}

// GetGroup returns consumer group details
func (h *ConsumerGroupHandler) GetGroup(c *gin.Context) {
	id := c.Param("id")

	group, err := h.consumerGroupService.GetGroup(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
}

// GetGroupLag returns consumer group lag
func (h *ConsumerGroupHandler) GetGroupLag(c *gin.Context) {
	id := c.Param("id")

	lag, err := h.consumerGroupService.GetGroupLag(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, lag)
}

// ResetOffset resets consumer group offset
func (h *ConsumerGroupHandler) ResetOffset(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Topic     string `json:"topic" binding:"required"`
		Partition int    `json:"partition" binding:"required"`
		Offset    int64  `json:"offset" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.consumerGroupService.ResetOffset(c.Request.Context(), id, req.Topic, req.Partition, req.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "offset reset successfully"})
}
