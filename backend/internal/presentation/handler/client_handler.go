package handler

import (
	"context"

	"github.com/clic_newlife/backend/internal/application/usecase"
	"github.com/gofiber/fiber/v2"
)

func GetClientData(uc *usecase.FetchClientDataUseCase) fiber.Handler {
	return func(c *fiber.Ctx) error {
		cpf := c.Params("cpf")
		if cpf == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "CPF is required"})
		}

		ctx := context.Background()
		data, err := uc.Execute(ctx, cpf)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(data)
	}
}
