package handler

import (
	"math"
	"regexp"

	"github.com/clic_newlife/backend/internal/application/usecase"
	"github.com/gofiber/fiber/v2"
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
	return c.Render("index", fiber.Map{}, "layouts/main")
}

// TemplateData represents the data passed to the dashboard template
type TemplateData struct {
	Error             string
	Client            interface{}
	Conexoes          interface{}
	Contratos         interface{}
	Faturas           interface{}
	Atendimentos      interface{}
	Financeiro        interface{}
	ScorePercentage   int
	HasBlockedConexao bool
	OpenAtendimentos  int
}

// HandleSearch processes the form submission via HTMX
func (h *UIHandler) HandleSearch(c *fiber.Ctx) error {
	cpf := c.FormValue("cpf")
	if cpf == "" {
		// Fallback if someone hits GET /search or missing param
		return c.Render("index", fiber.Map{"Error": "CPF/CNPJ é obrigatório"}, "layouts/main")
	}

	// Clean CPF (only digits)
	re := regexp.MustCompile(`\D`)
	cleanCpf := re.ReplaceAllString(cpf, "")

	data, err := h.fetchClientDataUC.Execute(c.Context(), cleanCpf)
	
	// Prepare template data
	tmplData := TemplateData{}
	
	if err != nil {
		tmplData.Error = err.Error()
		if tmplData.Error == "" {
			tmplData.Error = "Erro ao buscar cliente."
		}
		// If HTMX request, we can just return the index partial with the error inside it
		if c.Get("HX-Request") == "true" {
			return c.Render("index", tmplData)
		}
		return c.Render("index", tmplData, "layouts/main")
	}

	tmplData.Client = data.Client
	tmplData.Conexoes = data.Conexoes
	tmplData.Contratos = data.Contratos
	tmplData.Faturas = data.Faturas
	tmplData.Atendimentos = data.Atendimentos
	tmplData.Financeiro = data.Financeiro

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
