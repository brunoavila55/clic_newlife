package handler

import (
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/clic_newlife/backend/internal/application/usecase"
	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/domain"
	"github.com/clic_newlife/backend/internal/infrastructure/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// UIHandler represents the HTTP handler for the frontend UI
type UIHandler struct {
	fetchClientDataUC *usecase.FetchClientDataUseCase
}

func NewUIHandler(fetchClientDataUC *usecase.FetchClientDataUseCase) *UIHandler {
	return &UIHandler{fetchClientDataUC: fetchClientDataUC}
}

// RenderIndex renders the initial search page
func (h *UIHandler) RenderIndex(c *fiber.Ctx) error {
	return c.Render("index", fiber.Map{
		"CurrentUser": c.Locals("user"),
	}, "layouts/main")
}

// TemplateData represents the data passed to the dashboard template
type TemplateData struct {
	Error             string
	Client            interface{}
	Conexoes          interface{}
	Faturas           interface{}
	Atendimentos      interface{}
	Financeiro        interface{}
	Equipamentos      interface{}
	ScorePercentage   int
	HasBlockedConexao bool
	OpenAtendimentos  int
	CurrentUser       interface{}
}

// HandleSearch processes the form submission via HTMX
func (h *UIHandler) HandleSearch(c *fiber.Ctx) error {
	cpf := c.FormValue("cpf")
	currentUser := c.Locals("user")
	
	if cpf == "" {
		return c.Render("index", fiber.Map{
			"Error":       "CPF/CNPJ é obrigatório",
			"CurrentUser": currentUser,
		}, "layouts/main")
	}

	// Clean CPF (only digits)
	re := regexp.MustCompile(`\D`)
	cleanCpf := re.ReplaceAllString(cpf, "")

	data, err := h.fetchClientDataUC.Execute(c.Context(), cleanCpf)
	
	// Prepare template data
	tmplData := TemplateData{
		CurrentUser: currentUser,
	}
	
	if err != nil {
		tmplData.Error = err.Error()
		if tmplData.Error == "" {
			tmplData.Error = "Erro ao buscar cliente."
		}
		if c.Get("HX-Request") == "true" {
			return c.Render("index", tmplData)
		}
		return c.Render("index", tmplData, "layouts/main")
	}

	tmplData.Client = data.Client
	tmplData.Conexoes = data.Conexoes
	tmplData.Faturas = data.Faturas
	tmplData.Atendimentos = data.Atendimentos
	tmplData.Financeiro = data.Financeiro
	tmplData.Equipamentos = data.Equipamentos

	// Calculate Score Percentage
	if data.Financeiro.CreditScore > 0 {
		pct := (float64(data.Financeiro.CreditScore) / 1000.0) * 100.0
		tmplData.ScorePercentage = int(math.Min(100, pct))
	}

	// Calculate HasBlockedConexao
	for _, con := range data.Conexoes {
		if con.Bloqueada == "Sim" {
			tmplData.HasBlockedConexao = true
			break
		}
	}

	// Calculate OpenAtendimentos
	for _, atd := range data.Atendimentos {
		if atd.Status == "OPEN" {
			tmplData.OpenAtendimentos++
		}
	}

	// Render HTMX partial without layout
	if c.Get("HX-Request") == "true" {
		return c.Render("dashboard", tmplData)
	}

	// Fallback for non-HTMX request
	return c.Render("dashboard", tmplData, "layouts/main")
}

// RenderLogin renders the login screen
func (h *UIHandler) RenderLogin(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "clic_session",
		Value:    "",
		Expires:  time.Now().Add(-24 * time.Hour),
		HTTPOnly: true,
		Path:     "/",
	})
	return c.Render("login", fiber.Map{}, "layouts/main")
}

// HandleLogin processes credentials, issues JWT and stores cookie
func (h *UIHandler) HandleLogin(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")

		var user domain.User
		if err := repository.DB.Where("username = ?", username).First(&user).Error; err != nil {
			return c.Render("login", fiber.Map{"Error": "Usuário ou senha incorretos."}, "layouts/main")
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
			return c.Render("login", fiber.Map{"Error": "Usuário ou senha incorretos."}, "layouts/main")
		}

		// Generate JWT token
		claims := jwt.MapClaims{
			"sub":  fmt.Sprintf("%d", user.ID),
			"role": user.Role,
			"exp":  time.Now().Add(time.Hour * 72).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		t, err := token.SignedString([]byte(cfg.JWTSecret))
		if err != nil {
			return c.Render("login", fiber.Map{"Error": "Erro interno ao criar sessão."}, "layouts/main")
		}

		// Set signed cookie
		c.Cookie(&fiber.Cookie{
			Name:     "clic_session",
			Value:    t,
			Expires:  time.Now().Add(time.Hour * 72),
			HTTPOnly: true,
			Secure:   false, // Set to true if running over HTTPS
			Path:     "/",
		})

		// Check if they need to setup password on first login
		if user.Username == "admin" && password == "admin" {
			return c.Redirect("/admin/setup-password")
		}

		return c.Redirect("/")
	}
}

// HandleLogout clears session cookies and redirects
func (h *UIHandler) HandleLogout(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "clic_session",
		Value:    "",
		Expires:  time.Now().Add(-24 * time.Hour),
		HTTPOnly: true,
		Path:     "/",
	})
	return c.Redirect("/login")
}

// RenderSetupPassword renders the administrator password setup screen
func (h *UIHandler) RenderSetupPassword(c *fiber.Ctx) error {
	user := c.Locals("user").(domain.User)
	return c.Render("setup_password", fiber.Map{
		"CurrentUser": user,
	}, "layouts/main")
}

// HandleSetupPassword updates the admin password and removes the force redirect
func (h *UIHandler) HandleSetupPassword(c *fiber.Ctx) error {
	user := c.Locals("user").(domain.User)

	if user.Username != "admin" {
		return c.Redirect("/")
	}

	password := c.FormValue("password")
	confirm := c.FormValue("confirm_password")

	if password == "" || confirm == "" {
		return c.Render("setup_password", fiber.Map{
			"Error":       "Ambos os campos são obrigatórios.",
			"CurrentUser": user,
		}, "layouts/main")
	}

	if password != confirm {
		return c.Render("setup_password", fiber.Map{
			"Error":       "As senhas não coincidem.",
			"CurrentUser": user,
		}, "layouts/main")
	}

	if password == "admin" {
		return c.Render("setup_password", fiber.Map{
			"Error":       "A nova senha não pode ser 'admin'. Escolha uma senha segura.",
			"CurrentUser": user,
		}, "layouts/main")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Render("setup_password", fiber.Map{
			"Error":       "Erro ao processar a nova senha.",
			"CurrentUser": user,
		}, "layouts/main")
	}

	// Update user password in DB and mark password as changed (setup complete)
	if err := repository.DB.Model(&user).Updates(map[string]interface{}{
		"password":         string(hashedPassword),
		"password_changed": true,
	}).Error; err != nil {
		return c.Render("setup_password", fiber.Map{
			"Error":       "Erro ao atualizar a senha no banco de dados.",
			"CurrentUser": user,
		}, "layouts/main")
	}

	return c.Redirect("/")
}

// RenderUsers lists all system accounts for Admin dashboard
func (h *UIHandler) RenderUsers(c *fiber.Ctx) error {
	var users []domain.User
	if err := repository.DB.Order("id desc").Find(&users).Error; err != nil {
		return c.Render("index", fiber.Map{
			"Error":       "Erro ao carregar usuários.",
			"CurrentUser": c.Locals("user"),
		}, "layouts/main")
	}

	return c.Render("users", fiber.Map{
		"Users":       users,
		"CurrentUser": c.Locals("user"),
	}, "layouts/main")
}

// HandleCreateUser inserts a new user
func (h *UIHandler) HandleCreateUser(c *fiber.Ctx) error {
	username := c.FormValue("username")
	name := c.FormValue("name")
	password := c.FormValue("password")
	role := c.FormValue("role")

	if username == "" || name == "" || password == "" || (role != "admin" && role != "user") {
		return c.Status(fiber.StatusBadRequest).SendString("Preencha todos os campos obrigatórios.")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Erro ao criptografar senha.")
	}

	user := domain.User{
		Username: username,
		Name:     name,
		Password: string(hashedPassword),
		Role:     role,
	}

	if err := repository.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Erro: Usuário já existe.")
	}

	c.Set("HX-Redirect", "/admin/users")
	return c.SendStatus(fiber.StatusOK)
}

// HandleDeleteUser deletes a user (prevents self deletion)
func (h *UIHandler) HandleDeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	currentUser := c.Locals("user").(domain.User)

	if fmt.Sprintf("%d", currentUser.ID) == id {
		return c.Status(fiber.StatusBadRequest).SendString("Você não pode excluir o seu próprio usuário.")
	}

	if err := repository.DB.Delete(&domain.User{}, id).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Erro ao excluir usuário.")
	}

	c.Set("HX-Redirect", "/admin/users")
	return c.SendStatus(fiber.StatusOK)
}
