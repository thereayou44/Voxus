package models

import (
	"github.com/google/uuid"
	"time"
)

type Room struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Name       string    `gorm:"not null"`
	Type       string    `gorm:"not null;check:type IN ('direct','group')"`
	MaxMembers int       `gorm:"default:20"`
	CreatedBy  uuid.UUID
	CreatedAt  time.Time

	// Связи
	Members  []User    `gorm:"many2many:room_members"`
	Messages []Message `gorm:"foreignKey:RoomID"`
}
