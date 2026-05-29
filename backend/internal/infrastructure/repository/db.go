package repository

import (
	"log"
	"os"
	"path/filepath"

	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/domain"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg *config.Config) {
	// Create data directory if it doesn't exist
	dbPath := "data/clic.db"
	if os.Getenv("NODE_ENV") != "development" && os.Getenv("PORT") == "8080" {
		// Just a heuristic to check if in docker, but we can safely just create the dir anywhere
		// The docker compose mounts /app/data
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	DB = db
	log.Println("Database connection established (SQLite)")

	// Auto Migrate ONLY User
	err = DB.AutoMigrate(&domain.User{})
	if err != nil {
		log.Fatalf("Failed to auto migrate: %v", err)
	}
	
	// Create default admin user if none exists
	var count int64
	DB.Model(&domain.User{}).Count(&count)
	if count == 0 {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		DB.Create(&domain.User{
			Email:    "admin@admin.com",
			Password: string(hashedPassword),
			Name:     "Admin User",
		})
		log.Println("Created default admin user (admin@admin.com / admin)")
	}
}
