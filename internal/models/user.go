package models

import (
	"github.com/google/uuid"
	"time"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Username     string    `gorm:"uniqueIndex;not null"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	AvatarURL    string
	LastSeenAt   time.Time
	CreatedAt    time.Time
}
