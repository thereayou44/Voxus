package handlers

import (
	"github.com/google/uuid"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/thereayou/discord-lite/internal/middleware"
	ws "github.com/thereayou/discord-lite/internal/websocket"
)

// WebSocketHandler управляет WebSocket соединениями
type WebSocketHandler struct {
	hub            *ws.Hub
	messageHandler *MessageHandler
	upgrader       websocket.Upgrader
}

// NewWebSocketHandler создает новый WebSocket handler
func NewWebSocketHandler(hub *ws.Hub, messageHandler *MessageHandler) *WebSocketHandler {
	return &WebSocketHandler{
		hub:            hub,
		messageHandler: messageHandler,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Проверить origin в prod
				return true
			},
		},
	}
}

// HandleWebSocket обрабатывает WebSocket соединения
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Получаем userID из контекста
	userID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := ws.NewClient(h.hub, conn, userID.(uuid.UUID))

	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump(h.messageHandler)
}
