package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gstreamio/streambus/ui/backend/services"
)

// TopicHandler handles topic-related requests
type TopicHandler struct {
	topicService *services.TopicService
}

// NewTopicHandler creates a new topic handler
func NewTopicHandler(topicService *services.TopicService) *TopicHandler {
	return &TopicHandler{
		topicService: topicService,
	}
}

// ListTopics returns all topics
func (h *TopicHandler) ListTopics(c *gin.Context) {
	topics, err := h.topicService.ListTopics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, topics)
}

// CreateTopic creates a new topic
func (h *TopicHandler) CreateTopic(c *gin.Context) {
	var req struct {
		Name              string            `json:"name" binding:"required"`
		Partitions        int               `json:"partitions" binding:"required,min=1"`
		ReplicationFactor int               `json:"replicationFactor" binding:"required,min=1"`
		Config            map[string]string `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.topicService.CreateTopic(c.Request.Context(), req.Name, req.Partitions, req.ReplicationFactor, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "topic created successfully"})
}

// GetTopic returns topic details
func (h *TopicHandler) GetTopic(c *gin.Context) {
	name := c.Param("name")

	topic, err := h.topicService.GetTopic(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, topic)
}

// DeleteTopic deletes a topic
func (h *TopicHandler) DeleteTopic(c *gin.Context) {
	name := c.Param("name")

	err := h.topicService.DeleteTopic(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "topic deleted successfully"})
}

// UpdateTopicConfig updates topic configuration
func (h *TopicHandler) UpdateTopicConfig(c *gin.Context) {
	name := c.Param("name")

	var req struct {
		Config map[string]string `json:"config" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.topicService.UpdateTopicConfig(c.Request.Context(), name, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "config updated successfully"})
}

// GetPartitions returns partitions for a topic
func (h *TopicHandler) GetPartitions(c *gin.Context) {
	name := c.Param("name")

	partitions, err := h.topicService.GetPartitions(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, partitions)
}

// GetMessages returns messages from a topic
func (h *TopicHandler) GetMessages(c *gin.Context) {
	name := c.Param("name")

	partitionStr := c.DefaultQuery("partition", "0")
	partition, _ := strconv.Atoi(partitionStr)

	offsetStr := c.DefaultQuery("offset", "0")
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)
	if limit > 1000 {
		limit = 1000
	}

	messages, err := h.topicService.GetMessages(c.Request.Context(), name, partition, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, messages)
}
