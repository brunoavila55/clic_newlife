package main

import (
	"log"

	"github.com/clic_newlife/backend/internal/application/usecase"
	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/infrastructure/integration"
	"github.com/clic_newlife/backend/internal/infrastructure/repository"
	"github.com/clic_newlife/backend/internal/presentation/handler"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
)

func main() {
	// Load config
	cfg := config.LoadConfig()

	// Init DB
	repository.InitDB(cfg)

	// Init template engine
	engine := html.New("./views", ".html")

	// Init app
	app := fiber.New(fiber.Config{
		Views: engine,
	})
	
	// Middleware
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Services
	mkService := integration.NewMKIntegrationService(cfg)

	// Usecases
	fetchClientDataUC := usecase.NewFetchClientDataUseCase(mkService)

	// UI Routes
	uiHandler := handler.NewUIHandler(fetchClientDataUC)
	
	// Public UI Routes
	app.Get("/login", uiHandler.RenderLogin)
	app.Post("/login", uiHandler.HandleLogin(cfg))
	app.Get("/logout", uiHandler.HandleLogout)

	// Protected UI Routes
	authMid := handler.AuthRequired(cfg)
	app.Get("/", authMid, uiHandler.RenderIndex)
	app.Post("/search", authMid, uiHandler.HandleSearch)
	app.Get("/admin/setup-password", authMid, uiHandler.RenderSetupPassword)
	app.Post("/admin/setup-password", authMid, uiHandler.HandleSetupPassword)

	// Admin User Panel UI Routes
	adminMid := handler.AdminRequired()
	app.Get("/admin/users", authMid, adminMid, uiHandler.RenderUsers)
	app.Post("/admin/users/create", authMid, adminMid, uiHandler.HandleCreateUser)
	app.Delete("/admin/users/delete/:id", authMid, adminMid, uiHandler.HandleDeleteUser)

	// Routes
	api := app.Group("/api")
	
	// Auth routes
	api.Post("/login", handler.Login(cfg))

	// Client routes
	api.Get("/client/:cpf", handler.GetClientData(fetchClientDataUC))

	// Start server
	log.Printf("Starting server on port %s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
