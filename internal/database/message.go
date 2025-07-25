package database

import (
	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/models"
	"time"
)

func (d *Database) SaveMessage(message *models.Message) error {
	return d.db.Create(message).Error
}

func (d *Database) GetMessage(id string) (*models.Message, error) {
	var message models.Message
	if err := d.db.First(&message, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

func (d *Database) UpdateMessage(message *models.Message) error {
	return d.db.Save(message).Error
}

func (d *Database) DeleteMessage(id string) error {
	return d.db.Delete(&models.Message{}, "id = ?", id).Error
}

// GetRoomMessages получает сообщения комнаты с пагинацией
func (d *Database) GetRoomMessages(roomID string, limit int, beforeID *uuid.UUID) ([]models.Message, error) {
	var messages []models.Message

	query := d.db.Where("room_id = ?", roomID)

	// Если указан beforeID, получаем сообщения до него
	if beforeID != nil {
		var beforeMsg models.Message
		if err := d.db.First(&beforeMsg, "id = ?", beforeID).Error; err == nil {
			query = query.Where("created_at < ?", beforeMsg.CreatedAt)
		}
	}

	err := query.
		Order("created_at DESC").
		Limit(limit).
		Preload("User").
		Find(&messages).Error

	if err != nil {
		return nil, err
	}

	// Разворачиваем порядок, чтобы старые сообщения были первыми
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (d *Database) GetUnreadMessages(userID string, roomID string, lastReadAt time.Time) ([]models.Message, error) {
	var messages []models.Message

	err := d.db.
		Where("room_id = ? AND created_at > ? AND user_id != ?", roomID, lastReadAt, userID).
		Order("created_at ASC").
		Preload("User").
		Find(&messages).Error

	return messages, err
}
