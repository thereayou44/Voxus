package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/middleware"
	"github.com/thereayou/discord-lite/internal/models"
	"github.com/thereayou/discord-lite/internal/websocket"
)

type InviteHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewInviteHandler(db *database.Database, hub *websocket.Hub) *InviteHandler {
	return &InviteHandler{db: db, hub: hub}
}

// CreateInvite создает новое приглашение для комнаты
func (h *InviteHandler) CreateInvite(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	roomUUID, err := uuid.Parse(roomID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room id"})
		return
	}

	// Парсим параметры запроса
	var req struct {
		MaxUses   *int    `json:"max_uses"`   // nil = unlimited
		ExpiresIn *string `json:"expires_in"` // "24h", "7d", "30m", etc
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Парсим время истечения
	var expiresIn *time.Duration
	if req.ExpiresIn != nil {
		duration, err := parseDuration(*req.ExpiresIn)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid duration format"})
			return
		}
		expiresIn = &duration
	}

	// Создаем инвайт
	invite, err := h.db.CreateInvite(roomUUID, userID, expiresIn, req.MaxUses)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, formatInviteResponse(invite))
}

// GetRoomInvites получает список активных инвайтов комнаты
func (h *InviteHandler) GetRoomInvites(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	roomUUID, err := uuid.Parse(roomID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room id"})
		return
	}

	// Проверяем доступ к комнате
	hasAccess, err := h.db.UserHasAccessToRoom(userID.String(), roomID)
	if err != nil || !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	invites, err := h.db.GetRoomInvites(roomUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get invites"})
		return
	}

	response := make([]gin.H, len(invites))
	for i, invite := range invites {
		response[i] = formatInviteResponse(&invite)
	}

	c.JSON(http.StatusOK, gin.H{"invites": response})
}

// GetMyInvites получает инвайты, созданные текущим пользователем
func (h *InviteHandler) GetMyInvites(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	invites, err := h.db.GetUserInvites(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get invites"})
		return
	}

	response := make([]gin.H, len(invites))
	for i, invite := range invites {
		response[i] = formatInviteResponse(&invite)
	}

	c.JSON(http.StatusOK, gin.H{"invites": response})
}

// GetInviteInfo получает информацию об инвайте по коду (публичный эндпоинт)
func (h *InviteHandler) GetInviteInfo(c *gin.Context) {
	code := c.Param("code")

	invite, err := h.db.GetInviteByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
		return
	}

	if !invite.IsValid() {
		c.JSON(http.StatusGone, gin.H{"error": "invite expired or invalid"})
		return
	}

	// Возвращаем публичную информацию о комнате
	c.JSON(http.StatusOK, gin.H{
		"code": invite.Code,
		"room": gin.H{
			"id":           invite.Room.ID,
			"name":         invite.Room.Name,
			"type":         invite.Room.Type,
			"member_count": len(invite.Room.Members),
		},
		"expires_at":     invite.ExpiresAt,
		"remaining_uses": invite.GetRemainingUses(),
	})
}

// UseInvite использует инвайт для присоединения к комнате
func (h *InviteHandler) UseInvite(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	code := c.Param("code")

	room, err := h.db.UseInvite(code, userID)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "invalid invite code" {
			status = http.StatusNotFound
		} else if err.Error() == "invite is expired or reached max uses" {
			status = http.StatusGone
		} else if err.Error() == "room is full" {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Отправляем уведомление через WebSocket о новом участнике
	h.hub.SendToRoom(room.ID, []byte(`{"type":"member_joined","user_id":"`+userID.String()+`"}`))

	c.JSON(http.StatusOK, gin.H{
		"message": "successfully joined room",
		"room":    formatRoomResponse(room),
	})
}

// RevokeInvite отзывает инвайт
func (h *InviteHandler) RevokeInvite(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	inviteID := c.Param("id")

	inviteUUID, err := uuid.Parse(inviteID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invite id"})
		return
	}

	if err := h.db.RevokeInvite(inviteUUID, userID); err != nil {
		if err.Error() == "no permission to revoke this invite" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke invite"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "invite revoked successfully"})
}

// GetInviteStats получает статистику использования инвайта
func (h *InviteHandler) GetInviteStats(c *gin.Context) {
	//userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	inviteID := c.Param("id")

	inviteUUID, err := uuid.Parse(inviteID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invite id"})
		return
	}

	//// Проверяем права доступа
	//var invite struct {
	//	CreatedBy uuid.UUID
	//	RoomID    uuid.UUID
	//}
	//// Здесь нужна проверка прав, упрощенно:
	//_ = userID // TODO: проверить права

	stats, err := h.db.GetInviteStats(inviteUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Вспомогательные функции

func formatInviteResponse(invite *models.RoomInvite) gin.H {
	response := gin.H{
		"id":         invite.ID,
		"code":       invite.Code,
		"room_id":    invite.RoomID,
		"created_by": invite.CreatedBy,
		"created_at": invite.CreatedAt,
		"uses":       invite.Uses,
		"max_uses":   invite.MaxUses,
		"expires_at": invite.ExpiresAt,
		"is_valid":   invite.IsValid(),
	}

	if invite.Room.ID != uuid.Nil {
		response["room_name"] = invite.Room.Name
	}

	if invite.Creator.ID != uuid.Nil {
		response["creator_name"] = invite.Creator.Username
	}

	remaining := invite.GetRemainingUses()
	if remaining != nil {
		response["remaining_uses"] = *remaining
	}

	timeUntilExpiry := invite.GetTimeUntilExpiry()
	if timeUntilExpiry != nil {
		response["expires_in_seconds"] = int(timeUntilExpiry.Seconds())
	}

	return response
}

func parseDuration(s string) (time.Duration, error) {
	// Поддерживаем форматы: "30m", "24h", "7d"
	if len(s) < 2 {
		return 0, errors.New("invalid duration")
	}

	unit := s[len(s)-1]
	value := s[:len(s)-1]

	var multiplier time.Duration
	switch unit {
	case 'm':
		multiplier = time.Minute
	case 'h':
		multiplier = time.Hour
	case 'd':
		multiplier = 24 * time.Hour
	default:
		// Пробуем распарсить стандартный формат Go
		return time.ParseDuration(s)
	}

	var num int
	if _, err := fmt.Sscanf(value, "%d", &num); err != nil {
		return 0, err
	}

	return time.Duration(num) * multiplier, nil
}
