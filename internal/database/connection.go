package database

import (
	"github.com/thereayou/discord-lite/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
)

func (d *Database) Connect() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=yourpass dbname=discord_lite port=5432 sslmode=disable"
	}

	var err error
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	err = db.AutoMigrate(&models.User{}, &models.Room{}, &models.Message{})
	if err != nil {
		return err
	}

	d.db = db

	return nil
}
