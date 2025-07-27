package handlers

import (
	"encoding/json"
	"github.com/thereayou/discord-lite/internal/handlers/dto"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/models"
	"github.com/thereayou/discord-lite/internal/websocket"
)

type MessageHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewMessageHandler(db *database.Database, hub *websocket.Hub) *MessageHandler {
	return &MessageHandler{
		db:  db,
		hub: hub,
	}
}

func (h *MessageHandler) HandleMessage(client *websocket.Client, msg *websocket.Message) error {
	switch msg.Type {
	case websocket.TypeMessage:
		return h.handleTextMessage(client, msg)

	case websocket.TypeMessageEdit:
		return h.handleMessageEdit(client, msg)

	case websocket.TypeMessageDelete:
		return h.handleMessageDelete(client, msg)

	default:
		log.Printf("Unknown message type: %s", msg.Type)
		return nil
	}
}

func (h *MessageHandler) handleTextMessage(client *websocket.Client, msg *websocket.Message) error {
	if msg.RoomID == nil {
		return websocket.ErrInvalidMessage
	}

	if !client.IsInRoom(*msg.RoomID) {
		return websocket.ErrUserNotInRoom
	}

	var payload dto.MessagePayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return err
	}

	if payload.Content == "" {
		return websocket.ErrInvalidMessage
	}

	msgType := "text"
	if payload.Type != "" {
		msgType = payload.Type
	}

	message := &models.Message{
		RoomID:    *msg.RoomID,
		UserID:    client.UserID,
		Content:   payload.Content,
		Type:      msgType,
		CreatedAt: time.Now(),
	}

	if err := h.db.SaveMessage(message); err != nil {
		log.Printf("Failed to save message: %v", err)
		return err
	}

	user, err := h.db.GetUser(client.UserID.String())
	if err != nil {
		log.Printf("Failed to get user info: %v", err)
		return err
	}

	response := dto.MessageResponse{
		ID:        message.ID,
		RoomID:    message.RoomID,
		UserID:    message.UserID,
		Content:   message.Content,
		Type:      message.Type,
		CreatedAt: message.CreatedAt,
		User: dto.UserInfo{
			ID:        user.ID,
			Username:  user.Username,
			AvatarURL: user.AvatarURL,
		},
	}

	wsMsg := websocket.Message{
		Type:      websocket.TypeMessage,
		RoomID:    msg.RoomID,
		UserID:    client.UserID,
		Timestamp: time.Now(),
	}

	responseData, err := json.Marshal(response)
	if err != nil {
		return err
	}
	wsMsg.Data = responseData

	msgData, err := json.Marshal(wsMsg)
	if err != nil {
		return err
	}

	h.hub.SendToRoom(*msg.RoomID, msgData)

	go h.db.UpdateLastSeen(client.UserID.String())

	return nil
}

func (h *MessageHandler) handleMessageEdit(client *websocket.Client, msg *websocket.Message) error {
	type EditPayload struct {
		MessageID uuid.UUID `json:"message_id"`
		Content   string    `json:"content"`
	}

	var payload EditPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return err
	}

	message, err := h.db.GetMessage(payload.MessageID.String())
	if err != nil {
		return err
	}

	if message.UserID != client.UserID {
		return websocket.ErrUnauthorized
	}

	now := time.Now()
	message.Content = payload.Content
	message.EditedAt = &now

	if err := h.db.UpdateMessage(message); err != nil {
		return err
	}

	// Отправляем обновление всем в комнате
	response := map[string]interface{}{
		"message_id": message.ID,
		"content":    message.Content,
		"edited_at":  message.EditedAt,
	}

	wsMsg := websocket.Message{
		Type:      websocket.TypeMessageEdit,
		RoomID:    &message.RoomID,
		UserID:    client.UserID,
		Timestamp: time.Now(),
	}

	responseData, _ := json.Marshal(response)
	wsMsg.Data = responseData

	msgData, _ := json.Marshal(wsMsg)
	h.hub.SendToRoom(message.RoomID, msgData)

	return nil
}

func (h *MessageHandler) handleMessageDelete(client *websocket.Client, msg *websocket.Message) error {
	type DeletePayload struct {
		MessageID uuid.UUID `json:"message_id"`
	}

	var payload DeletePayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return err
	}

	message, err := h.db.GetMessage(payload.MessageID.String())
	if err != nil {
		return err
	}

	if message.UserID != client.UserID {
		return websocket.ErrUnauthorized
	}

	if err := h.db.DeleteMessage(payload.MessageID.String()); err != nil {
		return err
	}

	// Уведомляем всех в комнате
	response := map[string]interface{}{
		"message_id": payload.MessageID,
	}

	wsMsg := websocket.Message{
		Type:      websocket.TypeMessageDelete,
		RoomID:    &message.RoomID,
		UserID:    client.UserID,
		Timestamp: time.Now(),
	}

	responseData, _ := json.Marshal(response)
	wsMsg.Data = responseData

	msgData, _ := json.Marshal(wsMsg)
	h.hub.SendToRoom(message.RoomID, msgData)

	return nil
}

func (h *MessageHandler) LoadRoomHistory(roomID uuid.UUID, limit int, beforeID *uuid.UUID) ([]dto.MessageResponse, error) {
	messages, err := h.db.GetRoomMessages(roomID.String(), limit, beforeID)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.MessageResponse, len(messages))
	for i, msg := range messages {
		user, _ := h.db.GetUser(msg.UserID.String())

		responses[i] = dto.MessageResponse{
			ID:        msg.ID,
			RoomID:    msg.RoomID,
			UserID:    msg.UserID,
			Content:   msg.Content,
			Type:      msg.Type,
			CreatedAt: msg.CreatedAt,
			EditedAt:  msg.EditedAt,
			User: dto.UserInfo{
				ID:        user.ID,
				Username:  user.Username,
				AvatarURL: user.AvatarURL,
			},
		}
	}

	return responses, nil
}
