package database

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/models"
	"gorm.io/gorm"
)

// CreateInvite создает новое приглашение
func (d *Database) CreateInvite(roomID, createdBy uuid.UUID, expiresIn *time.Duration, maxUses *int) (*models.RoomInvite, error) {
	// Проверяем, что создатель является участником комнаты
	var count int64
	err := d.db.Table("room_members").
		Where("user_id = ? AND room_id = ?", createdBy, roomID).
		Count(&count).Error
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, errors.New("user is not a member of this room")
	}

	// Генерируем уникальный код
	code, err := models.GenerateInviteCode()
	if err != nil {
		return nil, err
	}

	// Проверяем уникальность кода (маловероятно, но лучше перестраховаться)
	var existingCount int64
	d.db.Model(&models.RoomInvite{}).Where("code = ?", code).Count(&existingCount)
	if existingCount > 0 {
		// Рекурсивно пробуем сгенерировать новый код
		return d.CreateInvite(roomID, createdBy, expiresIn, maxUses)
	}

	invite := &models.RoomInvite{
		RoomID:    roomID,
		Code:      code,
		CreatedBy: createdBy,
		MaxUses:   maxUses,
	}

	// Устанавливаем время истечения
	if expiresIn != nil {
		expiresAt := time.Now().Add(*expiresIn)
		invite.ExpiresAt = &expiresAt
	}

	if err := d.db.Create(invite).Error; err != nil {
		return nil, err
	}

	// Загружаем связанные данные
	d.db.Preload("Room").Preload("Creator").First(invite, invite.ID)

	return invite, nil
}

// GetInviteByCode получает инвайт по коду
func (d *Database) GetInviteByCode(code string) (*models.RoomInvite, error) {
	var invite models.RoomInvite
	err := d.db.Preload("Room").Preload("Creator").
		Where("code = ?", code).
		First(&invite).Error
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

// GetRoomInvites получает все активные инвайты комнаты
func (d *Database) GetRoomInvites(roomID uuid.UUID) ([]models.RoomInvite, error) {
	var invites []models.RoomInvite
	err := d.db.Preload("Creator").
		Where("room_id = ?", roomID).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("created_at DESC").
		Find(&invites).Error
	return invites, err
}

// GetUserInvites получает инвайты, созданные пользователем
func (d *Database) GetUserInvites(userID uuid.UUID) ([]models.RoomInvite, error) {
	var invites []models.RoomInvite
	err := d.db.Preload("Room").
		Where("created_by = ?", userID).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("created_at DESC").
		Find(&invites).Error
	return invites, err
}

// UseInvite использует инвайт для добавления пользователя в комнату
func (d *Database) UseInvite(code string, userID uuid.UUID) (*models.Room, error) {
	var room models.Room

	err := d.db.Transaction(func(tx *gorm.DB) error {
		// Получаем инвайт с блокировкой
		var invite models.RoomInvite
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("code = ?", code).
			First(&invite).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("invalid invite code")
			}
			return err
		}

		// Проверяем валидность
		if !invite.IsValid() {
			return errors.New("invite is expired or reached max uses")
		}

		// Проверяем, не является ли пользователь уже участником
		var memberCount int64
		if err := tx.Table("room_members").
			Where("room_id = ? AND user_id = ?", invite.RoomID, userID).
			Count(&memberCount).Error; err != nil {
			return err
		}
		if memberCount > 0 {
			return errors.New("user is already a member of this room")
		}

		// Проверяем лимит участников комнаты
		var currentMembers int64
		var maxMembers int
		if err := tx.Table("rooms").
			Select("max_members").
			Where("id = ?", invite.RoomID).
			Scan(&maxMembers).Error; err != nil {
			return err
		}

		if err := tx.Table("room_members").
			Where("room_id = ?", invite.RoomID).
			Count(&currentMembers).Error; err != nil {
			return err
		}

		if currentMembers >= int64(maxMembers) {
			return errors.New("room is full")
		}

		// Добавляем пользователя в комнату
		if err := tx.Exec(
			"INSERT INTO room_members (user_id, room_id) VALUES (?, ?)",
			userID, invite.RoomID,
		).Error; err != nil {
			return err
		}

		// Увеличиваем счетчик использований
		if err := tx.Model(&invite).
			Update("uses", gorm.Expr("uses + 1")).Error; err != nil {
			return err
		}

		// Записываем использование инвайта
		inviteUse := models.InviteUse{
			InviteID: invite.ID,
			UserID:   userID,
		}
		if err := tx.Create(&inviteUse).Error; err != nil {
			return err
		}

		// Получаем информацию о комнате
		if err := tx.Preload("Members").
			First(&room, invite.RoomID).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &room, nil
}

// RevokeInvite отзывает инвайт
func (d *Database) RevokeInvite(inviteID uuid.UUID, userID uuid.UUID) error {
	// Проверяем права на отзыв (создатель инвайта или создатель комнаты)
	var invite models.RoomInvite
	if err := d.db.First(&invite, inviteID).Error; err != nil {
		return err
	}

	var room models.Room
	if err := d.db.First(&room, invite.RoomID).Error; err != nil {
		return err
	}

	// Проверяем права
	if invite.CreatedBy != userID && room.CreatedBy != userID {
		return errors.New("no permission to revoke this invite")
	}

	// Удаляем инвайт
	return d.db.Delete(&invite).Error
}

// CleanupExpiredInvites удаляет истекшие инвайты
func (d *Database) CleanupExpiredInvites() error {
	return d.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&models.RoomInvite{}).Error
}

// GetInviteStats получает статистику по инвайту
func (d *Database) GetInviteStats(inviteID uuid.UUID) (map[string]interface{}, error) {
	var invite models.RoomInvite
	if err := d.db.Preload("UsedBy").First(&invite, inviteID).Error; err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"code":              invite.Code,
		"uses":              invite.Uses,
		"max_uses":          invite.MaxUses,
		"remaining_uses":    invite.GetRemainingUses(),
		"expires_at":        invite.ExpiresAt,
		"time_until_expiry": invite.GetTimeUntilExpiry(),
		"created_at":        invite.CreatedAt,
		"users_joined":      len(invite.UsedBy),
	}

	return stats, nil
}
