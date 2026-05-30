package handler

import (
	"fmt"
	"strconv"

	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/domain"
	"github.com/clic_newlife/backend/internal/infrastructure/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// AuthRequired protects UI routes with cookie-based JWT authentication
func AuthRequired(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		cookie := c.Cookies("clic_session")
		if cookie == "" {
			return handleAuthFailure(c)
		}

		token, err := jwt.Parse(cookie, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			return handleAuthFailure(c)
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return handleAuthFailure(c)
		}

		sub, err := claims.GetSubject()
		if err != nil || sub == "" {
			return handleAuthFailure(c)
		}

		userID, err := strconv.ParseUint(sub, 10, 32)
		if err != nil {
			return handleAuthFailure(c)
		}

		var user domain.User
		if err := repository.DB.First(&user, userID).Error; err != nil {
			return handleAuthFailure(c)
		}

		c.Locals("user", user)

		// Force admin to set a new password only if it was never changed (first setup)
		if user.Username == "admin" && !user.PasswordChanged {
			if c.Path() != "/admin/setup-password" && c.Path() != "/logout" {
				if c.Get("HX-Request") == "true" {
					c.Set("HX-Redirect", "/admin/setup-password")
					return c.SendStatus(fiber.StatusSeeOther)
				}
				return c.Redirect("/admin/setup-password")
			}
		}

		return c.Next()
	}
}

// AdminRequired ensures only administrators access the route
func AdminRequired() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := c.Locals("user")
		if u == nil {
			return c.Status(fiber.StatusForbidden).Render("index", fiber.Map{
				"Error": "Acesso não autorizado.",
			})
		}
		user := u.(domain.User)
		if user.Role != "admin" {
			if c.Get("HX-Request") == "true" {
				return c.Render("index", fiber.Map{
					"Error": "Acesso negado. Apenas administradores podem gerenciar usuários.",
				})
			}
			return c.Status(fiber.StatusForbidden).Render("index", fiber.Map{
				"Error": "Acesso negado. Apenas administradores podem gerenciar usuários.",
			}, "layouts/main")
		}
		return c.Next()
	}
}

func handleAuthFailure(c *fiber.Ctx) error {
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/login")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Redirect("/login")
}
