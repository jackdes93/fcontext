package main

import (
	"net/http"
	"strconv"

	httpserver "github.com/binhdp/example/plugins/http-server"
	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
	"github.com/binhdp/mqtt-forwarder/plugins/storage"
)

type GetMessagesRequest struct {
	Topic string `json:"topic" binding:"required"`
	Limit int    `json:"limit" binding:"max=1000"`
}

type GetMessagesResponse struct {
	Topic    string                    `json:"topic"`
	Messages []map[string]interface{}  `json:"messages"`
	Count    int                       `json:"count"`
}

// RegisterAPIRoutes đăng ký API routes
func RegisterAPIRoutes(s fcontext.ServiceContext) httpserver.RouteRegistrar {
	return func(r *gin.Engine) {
		api := r.Group("/api/v1")

		// Health check
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		// Get messages by topic
		api.GET("/messages/:topic", func(c *gin.Context) {
			topic := c.Param("topic")
			limitStr := c.DefaultQuery("limit", "100")
			limit, _ := strconv.Atoi(limitStr)

			st := s.MustGet("storage").(*storage.StorageComponent)

			messages, err := st.GetMessages(c, topic, limit)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}

			// Convert to response format
			resp := make([]map[string]interface{}, 0, len(messages))
			for _, msg := range messages {
				resp = append(resp, map[string]interface{}{
					"topic":     msg.Topic,
					"payload":   msg.Payload,
					"timestamp": msg.Timestamp,
				})
			}

			c.JSON(http.StatusOK, gin.H{
				"topic":    topic,
				"messages": resp,
				"count":    len(resp),
			})
		})

		// Get message count
		api.GET("/stats/:topic", func(c *gin.Context) {
			topic := c.Param("topic")
			
			st := s.MustGet("storage").(*storage.StorageComponent)
			pg := st.GetPostgresClient()

			if pg == nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "postgres not available",
				})
				return
			}

			count, err := pg.GetMessageCountByTopic(c, topic)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"topic": topic,
				"count": count,
			})
		})
	}
}
