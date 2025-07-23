package services

import "github.com/thereayou/discord-lite/internal/models"

type DatabaseService interface {
	Connect() error
	SaveUser(user *models.User) error
	GetUser(id string) (*models.User, error)
	UpdateUser(user *models.User) error
}
