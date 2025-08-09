package database

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"os"
)

func (d *Database) Connect() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/discord_lite?sslmode=disable"
	}

	log.Printf("Attempting to connect to: %s", dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	d.db = db
	log.Println("Database connected successfully")
	return nil
}

func (d *Database) DB() *gorm.DB {
	return d.db
}
