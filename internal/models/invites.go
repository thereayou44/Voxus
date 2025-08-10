package models

import (
	"crypto/rand"
	"encoding/base32"
	"github.com/google/uuid"
	"strings"
	"time"
)

type RoomInvite struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	RoomID    uuid.UUID  `gorm:"type:uuid;not null"`
	Code      string     `gorm:"unique;not null"`
	CreatedBy uuid.UUID  `gorm:"type:uuid;not null"`
	ExpiresAt *time.Time `gorm:"type:timestamp"`
	MaxUses   *int       `gorm:"type:int"`
	Uses      int        `gorm:"type:int;default:0"`
	CreatedAt time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP"`

	// Связи
	Room    Room   `gorm:"foreignKey:RoomID"`
	Creator User   `gorm:"foreignKey:CreatedBy"`
	UsedBy  []User `gorm:"many2many:invite_uses"`
}

type InviteUse struct {
	ID       uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	InviteID uuid.UUID `gorm:"type:uuid;not null"`
	UserID   uuid.UUID `gorm:"type:uuid;not null"`
	UsedAt   time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP"`

	// Связи
	Invite RoomInvite `gorm:"foreignKey:InviteID"`
	User   User       `gorm:"foreignKey:UserID"`
}

// GenerateInviteCode генерирует уникальный код приглашения
func GenerateInviteCode() (string, error) {
	// Генерируем 10 случайных байт
	randomBytes := make([]byte, 10)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// Конвертируем в base32 без паддинга
	code := strings.TrimRight(base32.StdEncoding.EncodeToString(randomBytes), "=")

	// Делаем код более читаемым, разделяя дефисами
	if len(code) > 8 {
		code = code[:4] + "-" + code[4:8] + "-" + code[8:]
	}

	return code, nil
}

// IsValid проверяет валидность инвайта
func (i *RoomInvite) IsValid() bool {
	// Проверяем срок действия
	if i.ExpiresAt != nil && i.ExpiresAt.Before(time.Now()) {
		return false
	}

	// Проверяем количество использований
	if i.MaxUses != nil && i.Uses >= *i.MaxUses {
		return false
	}

	return true
}

// GetRemainingUses возвращает количество оставшихся использований
func (i *RoomInvite) GetRemainingUses() *int {
	if i.MaxUses == nil {
		return nil // Безлимитный инвайт
	}

	remaining := *i.MaxUses - i.Uses
	return &remaining
}

// GetTimeUntilExpiry возвращает время до истечения инвайта
func (i *RoomInvite) GetTimeUntilExpiry() *time.Duration {
	if i.ExpiresAt == nil {
		return nil // Не истекает
	}

	duration := time.Until(*i.ExpiresAt)
	return &duration
}
