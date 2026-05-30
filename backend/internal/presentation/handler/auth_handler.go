package handler

import (
	"fmt"
	"time"

	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/domain"
	"github.com/clic_newlife/backend/internal/infrastructure/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func Login(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}

		var user domain.User
		if err := repository.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
		}

		// Create JWT token
		claims := jwt.MapClaims{
			"sub":  fmt.Sprintf("%d", user.ID),
			"role": user.Role,
			"exp":  time.Now().Add(time.Hour * 72).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		t, err := token.SignedString([]byte(cfg.JWTSecret))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not login"})
		}

		return c.JSON(fiber.Map{
			"token": t,
			"user": fiber.Map{
				"id":       user.ID,
				"username": user.Username,
				"name":     user.Name,
				"role":     user.Role,
			},
		})
	}
}
