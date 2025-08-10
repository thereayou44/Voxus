package handlers

import (
	"encoding/json"
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

type RoomHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewRoomHandler(db *database.Database, hub *websocket.Hub) *RoomHandler {
	return &RoomHandler{db: db, hub: hub}
}

// CreateRoom создает новую комнату
func (h *RoomHandler) CreateRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	var req struct {
		Name       string   `json:"name" binding:"required"`
		Type       string   `json:"type" binding:"required,oneof=group direct"`
		MemberIDs  []string `json:"member_ids"`
		MaxMembers int      `json:"max_members"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	maxMembers := req.MaxMembers
	if maxMembers == 0 {
		maxMembers = 20
	}

	room := &models.Room{
		Name:       req.Name,
		Type:       req.Type,
		MaxMembers: maxMembers,
		CreatedBy:  userID,
		CreatedAt:  time.Now(),
	}

	if err := h.db.CreateRoom(room); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create room"})
		return
	}

	// Добавляем создателя в комнату
	if err := h.db.AddUserToRoom(userID.String(), room.ID.String()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add creator to room"})
		return
	}

	// Добавляем других участников
	for _, memberID := range req.MemberIDs {
		if memberID != userID.String() {
			h.db.AddUserToRoom(memberID, room.ID.String())
		}
	}

	// Загружаем полную информацию о комнате
	fullRoom, _ := h.db.GetRoom(room.ID.String())

	c.JSON(http.StatusCreated, formatRoomResponse(fullRoom))
}

// CreateDirectRoom создает или получает direct комнату между двумя пользователями
func (h *RoomHandler) CreateDirectRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if userID == targetUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot create direct room with yourself"})
		return
	}

	room, err := h.db.GetOrCreateDirectRoom(userID, targetUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create direct room"})
		return
	}

	c.JSON(http.StatusOK, formatRoomResponse(room))
}

// GetMyRooms получает список комнат пользователя
func (h *RoomHandler) GetMyRooms(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	rooms, err := h.db.GetUserRooms(userID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get rooms"})
		return
	}

	// Добавляем информацию о последних сообщениях и количестве участников онлайн
	roomsResponse := make([]gin.H, len(rooms))
	for i, room := range rooms {
		roomResponse := formatRoomResponse(&room)

		// Получаем последнее сообщение
		messages, _ := h.db.GetRoomMessages(room.ID.String(), 1, nil)
		if len(messages) > 0 {
			roomResponse["last_message"] = gin.H{
				"id":         messages[0].ID,
				"content":    messages[0].Content,
				"user_id":    messages[0].UserID,
				"created_at": messages[0].CreatedAt,
			}
		}

		// Получаем количество участников онлайн
		onlineUsers := h.hub.GetRoomUsers(room.ID)
		roomResponse["online_count"] = len(onlineUsers)

		roomsResponse[i] = roomResponse
	}

	c.JSON(http.StatusOK, gin.H{"rooms": roomsResponse})
}

// GetRoom получает информацию о конкретной комнате
func (h *RoomHandler) GetRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем, что пользователь состоит в комнате
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

	response := formatRoomResponse(room)
	response["online_users"] = h.hub.GetRoomUsers(room.ID)

	c.JSON(http.StatusOK, response)
}

// UpdateRoom обновляет информацию о комнате
func (h *RoomHandler) UpdateRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем права (только создатель может обновлять)
	if room.CreatedBy != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only room creator can update room"})
		return
	}

	var req struct {
		Name       string `json:"name"`
		MaxMembers int    `json:"max_members"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Обновляем только переданные поля
	if req.Name != "" {
		room.Name = req.Name
	}
	if req.MaxMembers > 0 {
		room.MaxMembers = req.MaxMembers
	}

	if err := h.db.UpdateRoom(room); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update room"})
		return
	}

	c.JSON(http.StatusOK, formatRoomResponse(room))
}

// DeleteRoom удаляет комнату
func (h *RoomHandler) DeleteRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Только создатель может удалить комнату
	if room.CreatedBy != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only room creator can delete room"})
		return
	}

	if err := h.db.DeleteRoom(roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete room"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "room deleted successfully"})
}

// JoinRoom добавляет пользователя в комнату
func (h *RoomHandler) JoinRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем тип комнаты
	if room.Type == "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot join direct room"})
		return
	}

	// Проверяем лимит участников
	if len(room.Members) >= room.MaxMembers {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room is full"})
		return
	}

	if err := h.db.AddUserToRoom(userID.String(), roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to join room"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "joined room successfully"})
}

// LeaveRoom удаляет пользователя из комнаты
func (h *RoomHandler) LeaveRoom(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Нельзя покинуть direct комнату
	if room.Type == "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot leave direct room"})
		return
	}

	// Создатель не может покинуть комнату
	if room.CreatedBy == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room creator cannot leave room"})
		return
	}

	if err := h.db.RemoveUserFromRoom(userID.String(), roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to leave room"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "left room successfully"})
}

// GetRoomMembers получает список участников комнаты
func (h *RoomHandler) GetRoomMembers(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем, что пользователь состоит в комнате
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

	// Форматируем список участников
	members := make([]gin.H, len(room.Members))
	onlineUsers := h.hub.GetRoomUsers(room.ID)

	for i, member := range room.Members {
		isOnline := false
		for _, onlineID := range onlineUsers {
			if onlineID == member.ID {
				isOnline = true
				break
			}
		}

		members[i] = gin.H{
			"id":           member.ID,
			"username":     member.Username,
			"avatar_url":   member.AvatarURL,
			"last_seen_at": member.LastSeenAt,
			"is_online":    isOnline,
			"is_creator":   member.ID == room.CreatedBy,
		}
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (h *RoomHandler) AddMember(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверяем, что добавляемый пользователь существует
	targetUser, err := h.db.GetUser(req.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Получаем информацию о комнате
	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем права (только создатель может добавлять участников напрямую)
	// В будущем здесь можно добавить проверку ролей (админ, модератор)
	if room.CreatedBy != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only room owner can add members directly"})
		return
	}

	// Проверяем тип комнаты
	if room.Type == "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot add members to direct room"})
		return
	}

	// Проверяем, не является ли пользователь уже участником
	for _, member := range room.Members {
		if member.ID.String() == req.UserID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user is already a member"})
			return
		}
	}

	// Проверяем лимит участников
	if len(room.Members) >= room.MaxMembers {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room is full"})
		return
	}

	// Добавляем участника
	if err := h.db.AddUserToRoom(req.UserID, roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add member"})
		return
	}

	// Отправляем уведомление через WebSocket
	notification := gin.H{
		"type":     "member_added",
		"room_id":  roomID,
		"user_id":  req.UserID,
		"added_by": userID,
	}

	if data, err := json.Marshal(notification); err == nil {
		h.hub.SendToRoom(room.ID, data)
		// Также отправляем уведомление добавленному пользователю
		h.hub.SendToUser(targetUser.ID, data)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "member added successfully",
		"user": gin.H{
			"id":         targetUser.ID,
			"username":   targetUser.Username,
			"avatar_url": targetUser.AvatarURL,
		},
	})
}

// RemoveMember удаляет участника из комнаты (кик)
func (h *RoomHandler) RemoveMember(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")
	memberID := c.Param("memberId")

	// Получаем информацию о комнате
	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем права (только создатель может удалять участников)
	if room.CreatedBy != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only room owner can remove members"})
		return
	}

	// Проверяем тип комнаты
	if room.Type == "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove members from direct room"})
		return
	}

	// Нельзя удалить создателя комнаты
	if memberID == room.CreatedBy.String() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove room owner"})
		return
	}

	// Нельзя удалить самого себя через этот эндпоинт (используйте LeaveRoom)
	if memberID == userID.String() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "use leave endpoint to leave room"})
		return
	}

	// Проверяем, что пользователь является участником
	isMember := false
	for _, member := range room.Members {
		if member.ID.String() == memberID {
			isMember = true
			break
		}
	}

	if !isMember {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is not a member of this room"})
		return
	}

	// Удаляем участника
	if err := h.db.RemoveUserFromRoom(memberID, roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove member"})
		return
	}

	// Отправляем уведомление через WebSocket
	memberUUID, _ := uuid.Parse(memberID)
	notification := gin.H{
		"type":       "member_removed",
		"room_id":    roomID,
		"user_id":    memberID,
		"removed_by": userID,
	}

	if data, err := json.Marshal(notification); err == nil {
		h.hub.SendToRoom(room.ID, data)
		// Также отправляем уведомление удаленному пользователю
		h.hub.SendToUser(memberUUID, data)
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed successfully"})
}

// AddMembers добавляет несколько участников в комнату (массовое добавление)
func (h *RoomHandler) AddMembers(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	roomID := c.Param("id")

	var req struct {
		UserIDs []string `json:"user_ids" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Получаем информацию о комнате
	room, err := h.db.GetRoom(roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	// Проверяем права
	if room.CreatedBy != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only room owner can add members"})
		return
	}

	// Проверяем тип комнаты
	if room.Type == "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot add members to direct room"})
		return
	}

	// Проверяем лимит участников
	newTotalMembers := len(room.Members) + len(req.UserIDs)
	if newTotalMembers > room.MaxMembers {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("adding %d members would exceed room limit of %d",
				len(req.UserIDs), room.MaxMembers),
		})
		return
	}

	// Создаем мапу существующих участников для быстрой проверки
	existingMembers := make(map[string]bool)
	for _, member := range room.Members {
		existingMembers[member.ID.String()] = true
	}

	// Результаты добавления
	added := []string{}
	skipped := []string{}
	failed := []string{}

	// Добавляем каждого пользователя
	for _, userIDStr := range req.UserIDs {
		// Проверяем, что пользователь существует
		if _, err := h.db.GetUser(userIDStr); err != nil {
			failed = append(failed, userIDStr)
			continue
		}

		// Проверяем, не является ли уже участником
		if existingMembers[userIDStr] {
			skipped = append(skipped, userIDStr)
			continue
		}

		// Добавляем участника
		if err := h.db.AddUserToRoom(userIDStr, roomID); err != nil {
			failed = append(failed, userIDStr)
		} else {
			added = append(added, userIDStr)
		}
	}

	// Отправляем уведомления через WebSocket для успешно добавленных
	if len(added) > 0 {
		notification := gin.H{
			"type":     "members_added",
			"room_id":  roomID,
			"user_ids": added,
			"added_by": userID,
		}

		if data, err := json.Marshal(notification); err == nil {
			h.hub.SendToRoom(room.ID, data)

			// Отправляем уведомления добавленным пользователям
			for _, addedID := range added {
				if uid, err := uuid.Parse(addedID); err == nil {
					h.hub.SendToUser(uid, data)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "batch add completed",
		"added":   added,
		"skipped": skipped,
		"failed":  failed,
		"summary": gin.H{
			"total_requested": len(req.UserIDs),
			"added_count":     len(added),
			"skipped_count":   len(skipped),
			"failed_count":    len(failed),
		},
	})
}

// formatRoomResponse форматирует ответ для комнаты
func formatRoomResponse(room *models.Room) gin.H {
	members := make([]gin.H, len(room.Members))
	for i, member := range room.Members {
		members[i] = gin.H{
			"id":         member.ID,
			"username":   member.Username,
			"avatar_url": member.AvatarURL,
		}
	}

	return gin.H{
		"id":          room.ID,
		"name":        room.Name,
		"type":        room.Type,
		"max_members": room.MaxMembers,
		"created_by":  room.CreatedBy,
		"created_at":  room.CreatedAt,
		"members":     members,
	}
}
