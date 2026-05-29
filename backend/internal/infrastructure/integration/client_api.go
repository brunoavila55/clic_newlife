package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/domain"
)

type MKIntegrationService struct {
	cfg *config.Config
}

func NewMKIntegrationService(cfg *config.Config) *MKIntegrationService {
	return &MKIntegrationService{cfg: cfg}
}

// Auth response
type MKAuthResponse struct {
	Expire              string `json:"Expire"`
	LimiteUso           int    `json:"LimiteUso"`
	ServicosAutorizados []int  `json:"ServicosAutorizados"`
	Token               string `json:"Token"`
	Status              string `json:"status"`
}

// Client response
type MKClientResponse struct {
	CEP          string `json:"CEP"`
	CodigoPessoa int    `json:"CodigoPessoa"`
	Email        string `json:"Email"`
	Endereco     string `json:"Endereco"`
	Fone         string `json:"Fone"`
	Nome         string `json:"Nome"`
	Situacao     string `json:"Situacao"`
	Status       string `json:"status"`
}

// Conexoes response
type MKConexoesResponse struct {
	CodigoPessoa int `json:"CodigoPessoa"`
	Conexoes     []struct {
		Bloqueada      string  `json:"bloqueada"`
		Cadastro       string  `json:"cadastro"`
		Cep            string  `json:"cep"`
		CodConexao     int     `json:"codconexao"`
		Contrato       *int    `json:"contrato"`
		Endereco       string  `json:"endereco"`
		Latitude       string  `json:"latitude"`
		Longitude      string  `json:"longitude"`
		MacAddress     string  `json:"mac_address"`
		MotivoBloqueio *string `json:"motivo_bloqueio"`
		Username       string  `json:"username"`
	} `json:"Conexoes"`
	Nome   string `json:"Nome"`
	Status string `json:"status"`
}

func (s *MKIntegrationService) Authenticate(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/mk/WSAutenticacao.rule?sys=MK0&token=%s&cd_servico=9999", s.cfg.MKApiURL, s.cfg.MKAuthToken)
	if s.cfg.MKAuthPassword != "" {
		url += fmt.Sprintf("&password=%s", s.cfg.MKAuthPassword)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var authRes MKAuthResponse
	if err := json.Unmarshal(body, &authRes); err != nil {
		return "", fmt.Errorf("failed to parse auth response: %w. Body: %s", err, string(body))
	}

	if authRes.Token == "" {
		return "", fmt.Errorf("authentication failed, no token returned: %s", string(body))
	}

	return authRes.Token, nil
}

func (s *MKIntegrationService) FetchClientByCPF(ctx context.Context, sessionToken string, cpf string) (*domain.Client, error) {
	url := fmt.Sprintf("%s/mk/WSMKConsultaDoc.rule?sys=MK0&token=%s&doc=%s", s.cfg.MKApiURL, sessionToken, cpf)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var mkClient MKClientResponse
	if err := json.Unmarshal(body, &mkClient); err != nil {
		return nil, fmt.Errorf("failed to parse client response: %w. Body: %s", err, string(body))
	}

	if mkClient.Nome == "" {
		return nil, fmt.Errorf("cliente não encontrado ou resposta inválida")
	}

	type MKConexaoResponseV2 struct {
		Conexoes []struct {
			CodConexao     int     `json:"codconexao"`
			Username       string  `json:"username"`
			MacAddress     string  `json:"mac_address"`
			Bloqueada      string  `json:"bloqueada"`
			MotivoBloqueio *string `json:"motivo_bloqueio"`
			Endereco       string  `json:"endereco"`
		} `json:"Conexoes"`
	}

	return &domain.Client{
		CPF:        cpf,
		Name:       mkClient.Nome,
		Phone:      mkClient.Fone,
		Address:    mkClient.Endereco,
		InternalID: fmt.Sprintf("%d", mkClient.CodigoPessoa),
	}, nil
}

func (s *MKIntegrationService) FetchConexoes(ctx context.Context, sessionToken string, internalID string) ([]domain.Conexao, error) {
	url := fmt.Sprintf("%s/mk/WSMKConexoesPorCliente.rule?sys=MK0&token=%s&cd_cliente=%s", s.cfg.MKApiURL, sessionToken, internalID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// V1 returns a NESTED object: {"CodigoPessoa":..., "Conexoes":[...]}
	var mkRes MKConexoesResponse
	if err := json.Unmarshal(body, &mkRes); err != nil {
		fmt.Println("MK Conexoes Error:", string(body))
		return nil, fmt.Errorf("failed to parse conexoes: %w", err)
	}

	var conexoes []domain.Conexao
	for _, c := range mkRes.Conexoes {
		motivo := ""
		if c.MotivoBloqueio != nil {
			motivo = *c.MotivoBloqueio
		}

		con := domain.Conexao{
			CodConexao:     c.CodConexao,
			Username:       c.Username,
			MacAddress:     c.MacAddress,
			Bloqueada:      c.Bloqueada,
			MotivoBloqueio: motivo,
			Endereco:       c.Endereco,
		}

		// Enriquecer com dados de sessão (WSMKConsultaConexaoAutenticada)
		session := s.fetchConexaoSession(ctx, sessionToken, c.CodConexao)
		fmt.Printf("DEBUG Session for codconexao=%d: %+v\n", c.CodConexao, session)
		if session != nil {
			con.Down = session.Down
			con.Up = session.Up
			con.Tempo = session.Tempo
			con.NAS = session.NAS
		}

		conexoes = append(conexoes, con)
	}

	return conexoes, nil
}

// MKConexaoSession holds radius session data from WSMKConsultaConexaoAutenticada
type MKConexaoSession struct {
	Down  string
	Up    string
	Tempo string
	NAS   string
}

func (s *MKIntegrationService) fetchConexaoSession(ctx context.Context, sessionToken string, codConexao int) *MKConexaoSession {
	type MKAgendaItem struct {
		Down     string `json:"down"`
		Up       string `json:"up"`
		Tempo    string `json:"tempo"`
		Nas      string `json:"nas"`
		Username string `json:"username"`
	}
	type MKSessionResponse struct {
		Agendas []MKAgendaItem `json:"Agendas"`
		Status  string         `json:"status"`
	}

	url := fmt.Sprintf("%s/mk/WSMKConsultaConexaoAutenticada.rule?sys=MK0&token=%s&codconexao=%d", s.cfg.MKApiURL, sessionToken, codConexao)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var sesResp MKSessionResponse
	if err := json.Unmarshal(body, &sesResp); err != nil || len(sesResp.Agendas) == 0 {
		return nil
	}

	a := sesResp.Agendas[0]
	return &MKConexaoSession{
		Down:  a.Down,
		Up:    a.Up,
		Tempo: a.Tempo,
		NAS:   a.Nas,
	}
}

// Atendimento response array element
type MKAtendimento struct {
	CodigoAtendente   int    `json:"codigo_atendente"`
	CodigoAtendimento int    `json:"codigo_atendimento"`
	CodigoCliente     int    `json:"codigo_cliente"`
	CodigoStatus      string `json:"codigo_status"`
	Contratoid        *int   `json:"contratoid"`
	DataAbertura      string `json:"data_abertura"`
	DescricaoStatus   string `json:"descricao_status"`
	HrAbertura        string `json:"hr_abertura"`
	NomeAtendente     string `json:"nome_atendente"`
}

func (s *MKIntegrationService) FetchAtendimentos(ctx context.Context, sessionToken string, internalID string) ([]domain.Atendimento, error) {
	// Apenas últimos 30 dias para não sobrecarregar a API
	dataInicio := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	dataTermino := time.Now().Format("2006-01-02")

	url := fmt.Sprintf("%s/mk/WSMKAtendimentos.rule?sys=MK0&token=%s&data_inicio=%s&data_termino=%s", s.cfg.MKApiURL, sessionToken, dataInicio, dataTermino)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var mkRes []MKAtendimento
	if err := json.Unmarshal(body, &mkRes); err != nil {
		return nil, fmt.Errorf("failed to parse atendimentos: %w", err)
	}

	var atendimentos []domain.Atendimento
	for _, a := range mkRes {
		if fmt.Sprintf("%d", a.CodigoCliente) == internalID {
			t, _ := time.Parse("2006-01-02 15:04:05", a.DataAbertura+" "+a.HrAbertura)
			
			status := "CLOSED"
			if a.CodigoStatus != "F" {
				status = "OPEN"
			}

			subject := a.DescricaoStatus
			if a.NomeAtendente != "" {
				subject += " (" + a.NomeAtendente + ")"
			}

			atendimentos = append(atendimentos, domain.Atendimento{
				ID:        fmt.Sprintf("%d", a.CodigoAtendimento),
				Protocol:  fmt.Sprintf("%d", a.CodigoAtendimento),
				Status:    status,
				Subject:   subject,
				CreatedAt: t,
			})
		}
	}

	// Order descending
	for i := 0; i < len(atendimentos); i++ {
		for j := i + 1; j < len(atendimentos); j++ {
			if atendimentos[j].CreatedAt.After(atendimentos[i].CreatedAt) {
				atendimentos[i], atendimentos[j] = atendimentos[j], atendimentos[i]
			}
		}
	}

	return atendimentos, nil
}

func (s *MKIntegrationService) FetchFaturas(ctx context.Context, sessionToken string, internalID string) ([]domain.Fatura, error) {
	time.Sleep(600 * time.Millisecond)
	return []domain.Fatura{
		{ID: "F1", Amount: 150.00, DueDate: time.Now().AddDate(0, -1, 0), Status: "PAID", Barcode: "34191.09008 63571.277308"},
	}, nil
}

func (s *MKIntegrationService) FetchFinanceiro(ctx context.Context, sessionToken string, internalID string) (*domain.Financeiro, error) {
	time.Sleep(700 * time.Millisecond)
	return &domain.Financeiro{CreditScore: 850, TotalDebt: 0, IsDefaulter: false}, nil
}

func (s *MKIntegrationService) FetchContratos(ctx context.Context, sessionToken string, internalID string) ([]domain.Contrato, error) {
	time.Sleep(400 * time.Millisecond)
	return []domain.Contrato{
		{ID: "C1", PlanName: "Plano Ouro Fibra", Status: "ACTIVE", StartDate: time.Now().AddDate(-1, 0, 0)},
	}, nil
}
