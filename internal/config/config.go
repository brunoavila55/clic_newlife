package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	JWTSecret      string
	MKApiURL       string
	MKAuthToken    string
	MKAuthPassword string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		JWTSecret:      getEnv("JWT_SECRET", "supersecretjwtkey"),
		MKApiURL:       getEnv("MK_API_URL", "http://177.72.80.20:8080"),
		MKAuthToken:    getEnv("MK_AUTH_TOKEN", "f04160598fe6778cd26e35cf1ea71f15=3749e13d3b93220"),
		MKAuthPassword: getEnv("MK_AUTH_PASSWORD", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
