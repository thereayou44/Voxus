package database

import (
	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/models"
	"gorm.io/gorm"
	"time"
)

func (d *Database) CreateRoom(room *models.Room) error {
	return d.db.Create(room).Error
}

func (d *Database) GetRoom(id string) (*models.Room, error) {
	var room models.Room
	if err := d.db.Preload("Members").First(&room, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &room, nil
}

func (d *Database) GetUserRooms(userID string) ([]models.Room, error) {
	var user models.User
	err := d.db.Preload("Rooms").First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, err
	}

	// Для каждой комнаты загружаем участников
	for i := range user.Rooms {
		d.db.Model(&user.Rooms[i]).Association("Members").Find(&user.Rooms[i].Members)
	}

	return user.Rooms, nil
}

func (d *Database) AddUserToRoom(userID, roomID string) error {
	var user models.User
	var room models.Room

	if err := d.db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}

	if err := d.db.First(&room, "id = ?", roomID).Error; err != nil {
		return err
	}

	return d.db.Model(&room).Association("Members").Append(&user)
}

func (d *Database) RemoveUserFromRoom(userID, roomID string) error {
	var user models.User
	var room models.Room

	if err := d.db.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}

	if err := d.db.First(&room, "id = ?", roomID).Error; err != nil {
		return err
	}

	return d.db.Model(&room).Association("Members").Delete(&user)
}

func (d *Database) GetOrCreateDirectRoom(user1ID, user2ID uuid.UUID) (*models.Room, error) {
	var room models.Room

	// Ищем существующую direct комнату
	err := d.db.
		Joins("JOIN room_members rm1 ON rm1.room_id = rooms.id").
		Joins("JOIN room_members rm2 ON rm2.room_id = rooms.id").
		Where("rooms.type = 'direct' AND rm1.user_id = ? AND rm2.user_id = ?", user1ID, user2ID).
		First(&room).Error

	if err == nil {
		// Комната найдена
		return &room, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Создаем новую комнату
	room = models.Room{
		Name:      "Direct",
		Type:      "direct",
		CreatedBy: user1ID,
		CreatedAt: time.Now(),
	}

	if err := d.db.Create(&room).Error; err != nil {
		return nil, err
	}

	// Добавляем обоих пользователей
	if err := d.AddUserToRoom(user1ID.String(), room.ID.String()); err != nil {
		return nil, err
	}

	if err := d.AddUserToRoom(user2ID.String(), room.ID.String()); err != nil {
		return nil, err
	}

	d.db.Model(&room).Association("Members").Find(&room.Members)

	return &room, nil
}

func (d *Database) UpdateRoom(room *models.Room) error {
	return d.db.Save(room).Error
}

func (d *Database) DeleteRoom(id string) error {
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.Message{}, "room_id = ?", id).Error; err != nil {
			return err
		}

		var room models.Room
		if err := tx.First(&room, "id = ?", id).Error; err != nil {
			return err
		}

		if err := tx.Model(&room).Association("Members").Clear(); err != nil {
			return err
		}

		return tx.Delete(&room).Error
	})
}
