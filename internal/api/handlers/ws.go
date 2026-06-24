package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	internws "github.com/h2o/gps-platform/internal/websocket"
	"go.uber.org/zap"
)

// WSHandler upgrades HTTP to WebSocket for real-time GPS streaming
type WSHandler struct {
	hub *internws.Hub
	log *zap.Logger
}

func NewWSHandler(hub *internws.Hub, log *zap.Logger) *WSHandler {
	return &WSHandler{hub: hub, log: log}
}

// Handle upgrades the connection and starts streaming GPS events
func (h *WSHandler) Handle(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	if tenantID == "" || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	clientID := uuid.New().String()

	h.log.Info("websocket client connecting",
		zap.String("client", clientID),
		zap.String("tenant", tenantID),
		zap.String("user", userID))

	internws.ServeWS(h.hub, c.Writer, c.Request, clientID, tenantID, userID, h.log)
}
