package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
			con.IP = session.IP
			con.DtHrParada = session.DtHrParada
			con.DtHrRetorno = session.DtHrRetorno
		}

		conexoes = append(conexoes, con)
	}

	return conexoes, nil
}

// MKConexaoSession holds radius session data from WSMKConsultaConexaoAutenticada
type MKConexaoSession struct {
	Down        string
	Up          string
	Tempo       string
	NAS         string
	IP          string
	DtHrParada  string
	DtHrRetorno string
}

func (s *MKIntegrationService) fetchConexaoSession(ctx context.Context, sessionToken string, codConexao int) *MKConexaoSession {
	type MKAgendaItem struct {
		Down        string  `json:"down"`
		Up          string  `json:"up"`
		Tempo       string  `json:"tempo"`
		Nas         string  `json:"nas"`
		Username    string  `json:"username"`
		FramedIP    string  `json:"framedip"`
		DtHrParada  *string `json:"dt_hr_parada"`
		DtHrRetorno *string `json:"dt_hr_retorno"`
	}
	type MKSessionResponse struct {
		Agendas []MKAgendaItem `json:"Agendas"`
		Conexao *MKAgendaItem  `json:"conexao"`
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
	if err := json.Unmarshal(body, &sesResp); err != nil {
		return nil
	}

	var a MKAgendaItem
	if sesResp.Conexao != nil {
		a = *sesResp.Conexao
	} else if len(sesResp.Agendas) > 0 {
		a = sesResp.Agendas[0]
	} else {
		return nil
	}

	var parada, retorno string
	if a.DtHrParada != nil {
		parada = *a.DtHrParada
	}
	if a.DtHrRetorno != nil {
		retorno = *a.DtHrRetorno
	}

	return &MKConexaoSession{
		Down:        a.Down,
		Up:          a.Up,
		Tempo:       a.Tempo,
		NAS:         a.Nas,
		IP:          a.FramedIP,
		DtHrParada:  parada,
		DtHrRetorno: retorno,
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

			// Fetch OS Agendamento for each ticket
			osInfo := s.fetchOSAgendamento(ctx, sessionToken, internalID, a.DataAbertura)
			osStatus := "Sem O.S."
			osTecnico := ""
			osData := ""
			if osInfo != nil {
				osStatus = osInfo.OSStatus
				osTecnico = osInfo.OSTecnico
				osData = osInfo.OSData
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
				OSStatus:  osStatus,
				OSTecnico: osTecnico,
				OSData:    osData,
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

type MKOSAgendamento struct {
	OSStatus  string
	OSTecnico string
	OSData    string
}

func (s *MKIntegrationService) fetchOSAgendamento(ctx context.Context, sessionToken string, internalID string, dataAbertura string) *MKOSAgendamento {
	type MKOSItem struct {
		Cliente         string `json:"Cliente"`
		Data            string `json:"Data"`
		DataAgendamento string `json:"Data_agendamento"`
		DescricaoOS     string `json:"Descricao_os"`
		Protocolo       string `json:"Protocolo"`
		Tecnico         string `json:"Técnico"`
		Bairro          string `json:"bairro"`
		Cep             string `json:"cep"`
		CodCliente      int    `json:"codCliente"`
		CodContrato     *int   `json:"codcontrato"`
		CodOS           int    `json:"codos"`
		CodPessoa       int    `json:"codpessoa"`
		Logradouro      string `json:"logradouro"`
		SiglaEstado     string `json:"siglaestado"`
		Status          int    `json:"status"`
	}

	type MKOSResponse struct {
		Agendas []MKOSItem `json:"Agendas:"`
		Status  string     `json:"status"`
	}

	// dataAbertura is in format YYYY-MM-DD. Convert to DD/MM/AAAA.
	tData, err := time.Parse("2006-01-02", dataAbertura)
	formattedDate := dataAbertura
	if err == nil {
		formattedDate = tData.Format("02/01/2006")
	}

	url := fmt.Sprintf("%s/mk/WSMKConsultaOrdemAgendamento.rule?sys=MK0&token=%s&cliente=%s&DataAbertura=%s", 
		s.cfg.MKApiURL, sessionToken, internalID, formattedDate)

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

	var osResp MKOSResponse
	if err := json.Unmarshal(body, &osResp); err != nil {
		return nil
	}

	if len(osResp.Agendas) == 0 {
		return &MKOSAgendamento{
			OSStatus: "Sem O.S.",
		}
	}

	a := osResp.Agendas[0]
	status := "Não Agendada"
	if a.DataAgendamento != "" {
		status = "Agendada"
	}

	return &MKOSAgendamento{
		OSStatus:  status,
		OSTecnico: a.Tecnico,
		OSData:    a.DataAgendamento,
	}
}

func (s *MKIntegrationService) FetchFaturas(ctx context.Context, sessionToken string, internalID string) ([]domain.Fatura, error) {
	type MKFaturaItem struct {
		CdFatura int     `json:"cd_fatura"`
		Nome     string  `json:"nome"`
		Valor    float64 `json:"valor"`
	}
	type MKFaturasResponse struct {
		ListaFaturas []MKFaturaItem `json:"ListaFaturas"`
		Status       string         `json:"status"`
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	var faturas []domain.Fatura

	// Fetch ONLY OVERDUE invoices (from 01/01/2020 up to Yesterday)
	yesterdayStr := time.Now().AddDate(0, 0, -1).Format("02/01/2006")
	urlOverdue := fmt.Sprintf("%s/mk/WSMKFaturasAbertas.rule?sys=MK0&token=%s&dt_venc_inicio=%s&dt_venc_fim=%s&cd_pessoa=%s", 
		s.cfg.MKApiURL, sessionToken, "01/01/2020", yesterdayStr, internalID)

	reqOverdue, err := http.NewRequestWithContext(ctx, http.MethodGet, urlOverdue, nil)
	if err == nil {
		resp, err := httpClient.Do(reqOverdue)
		if err == nil {
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				var mkRes MKFaturasResponse
				if err := json.Unmarshal(body, &mkRes); err == nil {
					for _, item := range mkRes.ListaFaturas {
						faturas = append(faturas, domain.Fatura{
							ID:      fmt.Sprintf("%d", item.CdFatura),
							Amount:  item.Valor,
							DueDate: "Vencida",
							Status:  "OVERDUE",
						})
					}
				}
			}
		}
	}

	return faturas, nil
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

func (s *MKIntegrationService) FetchEquipamentos(ctx context.Context, sessionToken string, internalID string) ([]domain.Equipamento, error) {
	type MKStockItem struct {
		CodSetor       int     `json:"CodSetor"`
		Cod            int     `json:"cod"`
		ControleSerial string  `json:"controle_serial"`
		CustoTotal     float64 `json:"custo_total"`
		CustoUn        float64 `json:"custo_un"`
		Descricao      string  `json:"descricao"`
		DescricaoSetor string  `json:"descricao_setor"`
		EstoqueAtual   float64 `json:"estoque_atual"`
		Ordem          string  `json:"ordem"`
		VendaTotal     float64 `json:"venda_total"`
		VendaUn        float64 `json:"venda_un"`
	}

	type MKStockResponse struct {
		Codigos []MKStockItem `json:"Códigos:"`
		Codigo  []MKStockItem `json:"Código:"`
		Status  string        `json:"status"`
	}

	// Call the new active API WSMKConsultaProdutoEstoque.rule
	url := fmt.Sprintf("%s/mk/WSMKConsultaProdutoEstoque.rule?sys=MK0&Token=%s&cd_setor=%s", s.cfg.MKApiURL, sessionToken, internalID)

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

	var mkRes MKStockResponse
	if err := json.Unmarshal(body, &mkRes); err != nil {
		fmt.Printf("DEBUG: Error unmarshaling inventory response: %v. Body: %s\n", err, string(body))
		return nil, err
	}

	// Retrieve items from either list (dynamic key name support)
	items := mkRes.Codigos
	if len(items) == 0 {
		items = mkRes.Codigo
	}

	var equip []domain.Equipamento
	for _, item := range items {
		// Filter by description containing "roteador" or "ont" (case-insensitive)
		descLower := strings.ToLower(item.Descricao)
		if !strings.Contains(descLower, "roteador") && !strings.Contains(descLower, "ont") {
			continue
		}

		// Map serial label
		serialStr := "Sem Serial"
		if item.ControleSerial == "S" {
			serialStr = "Com Serial"
		} else if item.ControleSerial != "" && item.ControleSerial != "N" {
			serialStr = item.ControleSerial
		}

		equip = append(equip, domain.Equipamento{
			Codigo:           item.Cod,
			DescricaoProduto: item.Descricao,
			Status:           fmt.Sprintf("Qtd: %.0f", item.EstoqueAtual),
			Tipo:             item.DescricaoSetor,
			InSerial:         serialStr,
		})
	}

	return equip, nil
}
