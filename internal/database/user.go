package database

import (
	"github.com/thereayou/discord-lite/internal/models"
	"time"
)

func (d *Database) SaveUser(user *models.User) error {
	if err := d.db.Create(user).Error; err != nil {
		return err
	}
	return nil
}

func (d *Database) UpdateUser(user *models.User) error {
	return d.db.Save(user).Error
}

func (d *Database) GetUser(id string) (*models.User, error) {
	user := models.User{}
	if err := d.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *Database) FindUserByEmail(email string) (*models.User, error) {
	user := models.User{}
	if err := d.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *Database) SearchUsersByUsername(query string) ([]models.User, error) {
	var users []models.User
	err := d.db.Where("username ILIKE ?", "%"+query+"%").
		Limit(20).
		Find(&users).Error
	return users, err
}

func (d *Database) UpdateLastSeen(id string) error {
	return d.db.Model(&models.User{}).Where("id = ?", id).Update("last_seen_at", time.Now()).Error
}
