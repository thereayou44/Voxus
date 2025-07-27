package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/middleware"
	"github.com/thereayou/discord-lite/internal/models"
)

type HTTPMessageHandler struct {
	db *database.Database
}

func NewHTTPMessageHandler(db *database.Database) *HTTPMessageHandler {
	return &HTTPMessageHandler{db: db}
}

// GetRoomMessages получает историю сообщений комнаты
func (h *HTTPMessageHandler) GetRoomMessages(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	// Проверяем доступ к комнате
	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	isMember := false
	for _, member := range room.Members {
		if member.ID == userID {
			isMember = true
			break
		}
	}

	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "you are not a member of this room"})
		return
	}

	// Параметры пагинации
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var beforeID *uuid.UUID
	if before := c.Query("before"); before != "" {
		if id, err := uuid.Parse(before); err == nil {
			beforeID = &id
		}
	}

	// Получаем сообщения
	messages, err := h.db.GetRoomMessages(roomID, limit, beforeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get messages"})
		return
	}

	// Форматируем ответ
	result := make([]gin.H, len(messages))
	for i, msg := range messages {
		result[i] = formatMessageResponse(&msg)
	}

	c.JSON(http.StatusOK, gin.H{
		"messages": result,
		"has_more": len(messages) == limit,
	})
}

// SendMessage отправляет сообщение через HTTP (альтернатива WebSocket)
func (h *HTTPMessageHandler) SendMessage(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomIDStr := c.Param("id")

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room id"})
		return
	}

	// Проверяем доступ к комнате
	room, err := h.db.GetRoom(roomIDStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	isMember := false
	for _, member := range room.Members {
		if member.ID == userID {
			isMember = true
			break
		}
	}

	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "you are not a member of this room"})
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
		Type    string `json:"type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msgType := "text"
	if req.Type != "" {
		msgType = req.Type
	}

	message := &models.Message{
		RoomID:    roomID,
		UserID:    userID,
		Content:   req.Content,
		Type:      msgType,
		CreatedAt: time.Now(),
	}

	if err := h.db.SaveMessage(message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save message"})
		return
	}

	// Загружаем полную информацию о сообщении
	fullMessage, _ := h.db.GetMessage(message.ID.String())

	c.JSON(http.StatusCreated, formatMessageResponse(fullMessage))
}

// UpdateMessage обновляет сообщение
func (h *HTTPMessageHandler) UpdateMessage(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	messageID := c.Param("id")

	message, err := h.db.GetMessage(messageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}

	// Только автор может редактировать
	if message.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only edit your own messages"})
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	message.Content = req.Content
	message.EditedAt = &now

	if err := h.db.UpdateMessage(message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update message"})
		return
	}

	c.JSON(http.StatusOK, formatMessageResponse(message))
}

// DeleteMessage удаляет сообщение
func (h *HTTPMessageHandler) DeleteMessage(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	messageID := c.Param("id")

	message, err := h.db.GetMessage(messageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}

	// Только автор может удалять
	if message.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only delete your own messages"})
		return
	}

	if err := h.db.DeleteMessage(messageID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "message deleted successfully"})
}

// formatMessageResponse форматирует ответ для сообщения
func formatMessageResponse(msg *models.Message) gin.H {
	response := gin.H{
		"id":         msg.ID,
		"room_id":    msg.RoomID,
		"user_id":    msg.UserID,
		"content":    msg.Content,
		"type":       msg.Type,
		"created_at": msg.CreatedAt,
	}

	if msg.EditedAt != nil {
		response["edited_at"] = msg.EditedAt
	}

	// Если загружена информация о пользователе
	if msg.User.ID != uuid.Nil {
		response["user"] = gin.H{
			"id":         msg.User.ID,
			"username":   msg.User.Username,
			"avatar_url": msg.User.AvatarURL,
		}
	}

	return response
}
