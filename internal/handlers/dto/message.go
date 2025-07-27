package dto

import (
	"github.com/google/uuid"
	"time"
)

// MessagePayload структура для входящих сообщений
type MessagePayload struct {
	Content string `json:"content"`
	Type    string `json:"type,omitempty"` // text, image, file
}

// MessageResponse структура для исходящих сообщений
type MessageResponse struct {
	ID        uuid.UUID  `json:"id"`
	RoomID    uuid.UUID  `json:"room_id"`
	UserID    uuid.UUID  `json:"user_id"`
	Content   string     `json:"content"`
	Type      string     `json:"type"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at,omitempty"`
	User      UserInfo   `json:"user"`
}

type UserInfo struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url,omitempty"`
}
