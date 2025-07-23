package models

import (
	"github.com/google/uuid"
	"time"
)

type Message struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	RoomID    uuid.UUID `gorm:"not null"`
	UserID    uuid.UUID `gorm:"not null"`
	Content   string    `gorm:"not null"`
	Type      string    `gorm:"default:'text'"`
	CreatedAt time.Time
	EditedAt  *time.Time

	// Связи
	User User `gorm:"foreignKey:UserID"`
	Room Room `gorm:"foreignKey:RoomID"`
}
