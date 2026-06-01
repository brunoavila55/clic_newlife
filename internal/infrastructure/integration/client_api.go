package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/domain"
)

type MKIntegrationService struct {
	cfg *config.Config

	// httpClient otimizado para reaproveitar conexões TCP
	httpClient *http.Client

	// Token cache — evita uma autenticação por consulta
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func NewMKIntegrationService(cfg *config.Config) *MKIntegrationService {
	// Cria um transport focado em reaproveitamento (Pooling)
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 100

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: t,
	}

	return &MKIntegrationService{
		cfg:        cfg,
		httpClient: client,
	}
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

// Authenticate obtém o token de sessão MK, reutilizando o cache enquanto não expirar.
// Isso evita 1 requisição extra a cada pesquisa de cliente.
func (s *MKIntegrationService) Authenticate(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	if s.cachedToken != "" && time.Now().Before(s.tokenExpiry) {
		token := s.cachedToken
		s.tokenMu.Unlock()
		return token, nil
	}
	s.tokenMu.Unlock()

	url := fmt.Sprintf("%s/mk/WSAutenticacao.rule?sys=MK0&token=%s&cd_servico=9999", s.cfg.MKApiURL, s.cfg.MKAuthToken)
	if s.cfg.MKAuthPassword != "" {
		url += fmt.Sprintf("&password=%s", s.cfg.MKAuthPassword)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.httpClient.Do(req)
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

	// Armazena token em cache com margem de segurança de 2 minutos
	s.tokenMu.Lock()
	s.cachedToken = authRes.Token
	s.tokenExpiry = time.Now().Add(8 * time.Minute) // fallback conservador
	if authRes.Expire != "" {
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "01/02/2006 15:04:05"} {
			if t, parseErr := time.Parse(layout, authRes.Expire); parseErr == nil {
				s.tokenExpiry = t.Add(-2 * time.Minute)
				break
			}
		}
	}
	s.tokenMu.Unlock()

	return authRes.Token, nil
}

func (s *MKIntegrationService) FetchClientByCPF(ctx context.Context, sessionToken string, cpf string) (*domain.Client, error) {
	url := fmt.Sprintf("%s/mk/WSMKConsultaDoc.rule?sys=MK0&token=%s&doc=%s", s.cfg.MKApiURL, sessionToken, cpf)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
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

	return &domain.Client{
		CPF:        cpf,
		Name:       mkClient.Nome,
		Phone:      mkClient.Fone,
		Address:    mkClient.Endereco,
		InternalID: fmt.Sprintf("%d", mkClient.CodigoPessoa),
	}, nil
}

// MKConexaoSession holds radius session data from WSMKConsultaConexaoAutenticada
type MKConexaoSession struct {
	Down        string
	Up          string
	Tempo       string
	NAS         string
	IP          string
}

// FetchConexoes busca as conexões do cliente e enriquece cada uma com dados de sessão
// Radius em paralelo, reduzindo o tempo de N requisições sequenciais para 1x o tempo de
// uma única requisição (limitada pela mais lenta).
func (s *MKIntegrationService) FetchConexoes(ctx context.Context, sessionToken string, internalID string) ([]domain.Conexao, error) {
	url := fmt.Sprintf("%s/mk/WSMKConexoesPorCliente.rule?sys=MK0&token=%s&cd_cliente=%s", s.cfg.MKApiURL, sessionToken, internalID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var mkRes MKConexoesResponse
	if err := json.Unmarshal(body, &mkRes); err != nil {
		return nil, fmt.Errorf("failed to parse conexoes: %w", err)
	}

	if len(mkRes.Conexoes) == 0 {
		return nil, nil
	}

	// Busca sessões Radius em paralelo com limite de chamadas
	sessions := make([]*MKConexaoSession, len(mkRes.Conexoes))
	var sesWg sync.WaitGroup
	var sesMu sync.Mutex
	sem := make(chan struct{}, 5) // Semáforo de 5 requisições

	for i, c := range mkRes.Conexoes {
		sesWg.Add(1)
		go func(idx int, codConexao int) {
			defer sesWg.Done()
			sem <- struct{}{}        // Ocupa um slot
			defer func() { <-sem }() // Libera o slot
			session := s.fetchConexaoSession(ctx, sessionToken, codConexao)
			sesMu.Lock()
			sessions[idx] = session
			sesMu.Unlock()
		}(i, c.CodConexao)
	}
	sesWg.Wait()

	var conexoes []domain.Conexao
	for i, c := range mkRes.Conexoes {
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

		if sessions[i] != nil {
			con.Down = sessions[i].Down
			con.Up = sessions[i].Up
			con.Tempo = sessions[i].Tempo
			con.NAS = sessions[i].NAS
			con.IP = sessions[i].IP
		}

		conexoes = append(conexoes, con)
	}

	return conexoes, nil
}

func (s *MKIntegrationService) fetchConexaoSession(ctx context.Context, sessionToken string, codConexao int) *MKConexaoSession {
	type MKAgendaItem struct {
		Down        string  `json:"down"`
		Up          string  `json:"up"`
		Tempo       string  `json:"tempo"`
		Nas         string  `json:"nas"`
		Username    string  `json:"username"`
		FramedIP    string  `json:"framedip"`
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

	resp, err := s.httpClient.Do(req)
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



	return &MKConexaoSession{
		Down:        a.Down,
		Up:          a.Up,
		Tempo:       a.Tempo,
		NAS:         a.Nas,
		IP:          a.FramedIP,
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

// FetchAtendimentos busca atendimentos filtrando por cliente no servidor (cd_cliente),
// e busca os agendamentos de O.S. de todos os atendimentos em paralelo.
func (s *MKIntegrationService) FetchAtendimentos(ctx context.Context, sessionToken string, internalID string) ([]domain.Atendimento, error) {
	// Busca do último 1 ano para garantir histórico
	dataInicio := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	dataTermino := time.Now().Format("2006-01-02")

	url := fmt.Sprintf("%s/mk/WSMKAtendimentos.rule?sys=MK0&token=%s&cd_cliente=%s&data_inicio=%s&data_termino=%s",
		s.cfg.MKApiURL, sessionToken, internalID, dataInicio, dataTermino)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
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

	// Filtra pelo cliente (fallback caso a API ignore cd_cliente)
	type pendingAtd struct {
		base         domain.Atendimento
		dataAbertura string
	}

	var pending []pendingAtd
	for _, a := range mkRes {
		if fmt.Sprintf("%d", a.CodigoCliente) != internalID {
			continue
		}
		t, _ := time.Parse("2006-01-02 15:04:05", a.DataAbertura+" "+a.HrAbertura)

		status := "CLOSED"
		if a.CodigoStatus != "F" {
			status = "OPEN"
		}

		subject := a.DescricaoStatus
		if a.NomeAtendente != "" {
			subject += " (" + a.NomeAtendente + ")"
		}

		pending = append(pending, pendingAtd{
			base: domain.Atendimento{
				ID:        fmt.Sprintf("%d", a.CodigoAtendimento),
				Protocol:  fmt.Sprintf("%d", a.CodigoAtendimento),
				Status:    status,
				Subject:   subject,
				CreatedAt: t,
			},
			dataAbertura: a.DataAbertura,
		})
	}

	if len(pending) == 0 {
		return nil, nil
	}

	// Ordena pendentes por data decrescente
	for i := 0; i < len(pending); i++ {
		for j := i + 1; j < len(pending); j++ {
			if pending[j].base.CreatedAt.After(pending[i].base.CreatedAt) {
				pending[i], pending[j] = pending[j], pending[i]
			}
		}
	}

	// Limita aos 3 mais recentes ANTES de buscar as O.S (otimização)
	if len(pending) > 3 {
		pending = pending[:3]
	}

	// Busca agendamentos de O.S. em paralelo com controle de concorrência
	results := make([]domain.Atendimento, len(pending))
	var osWg sync.WaitGroup
	var osMu sync.Mutex
	sem := make(chan struct{}, 5)

	for i, p := range pending {
		osWg.Add(1)
		go func(idx int, atd domain.Atendimento, dataAbertura string) {
			defer osWg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			osInfo := s.fetchOSAgendamento(ctx, sessionToken, internalID, dataAbertura)
			atd.OSStatus = "Sem O.S."
			if osInfo != nil {
				atd.OSStatus = osInfo.OSStatus
				atd.OSTecnico = osInfo.OSTecnico
				atd.OSData = osInfo.OSData
			}
			osMu.Lock()
			results[idx] = atd
			osMu.Unlock()
		}(i, p.base, p.dataAbertura)
	}
	osWg.Wait()

	return results, nil
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

	resp, err := s.httpClient.Do(req)
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

	var faturas []domain.Fatura

	// Busca apenas faturas VENCIDAS (de 01/01/2020 até ontem)
	yesterdayStr := time.Now().AddDate(0, 0, -1).Format("02/01/2006")
	urlOverdue := fmt.Sprintf("%s/mk/WSMKFaturasAbertas.rule?sys=MK0&token=%s&dt_venc_inicio=%s&dt_venc_fim=%s&cd_pessoa=%s",
		s.cfg.MKApiURL, sessionToken, "01/01/2020", yesterdayStr, internalID)

	reqOverdue, err := http.NewRequestWithContext(ctx, http.MethodGet, urlOverdue, nil)
	if err == nil {
		resp, err := s.httpClient.Do(reqOverdue)
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

// FetchFinanceiro — integração pendente com MK Solutions.
func (s *MKIntegrationService) FetchFinanceiro(ctx context.Context, sessionToken string, internalID string) (*domain.Financeiro, error) {
	return &domain.Financeiro{CreditScore: 0, TotalDebt: 0, IsDefaulter: false}, nil
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

	url := fmt.Sprintf("%s/mk/WSMKConsultaProdutoEstoque.rule?sys=MK0&Token=%s&cd_setor=%s", s.cfg.MKApiURL, sessionToken, internalID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
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
		return nil, err
	}

	// Suporte a chave dinâmica (Códigos: ou Código:)
	items := mkRes.Codigos
	if len(items) == 0 {
		items = mkRes.Codigo
	}

	var equip []domain.Equipamento
	for _, item := range items {
		// Filtra apenas roteadores e ONTs
		descLower := strings.ToLower(item.Descricao)
		if !strings.Contains(descLower, "roteador") && !strings.Contains(descLower, "ont") {
			continue
		}

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
