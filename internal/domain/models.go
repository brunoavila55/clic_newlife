package domain

import (
	"time"

	"gorm.io/gorm"
)

// User represents a system user (for authentication)
type User struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Username        string         `gorm:"uniqueIndex;not null" json:"username"`
	Password        string         `gorm:"not null" json:"-"`
	Name            string         `json:"name"`
	Role            string         `gorm:"default:'user';not null" json:"role"` // "admin" or "user"
	PasswordChanged bool           `gorm:"default:false" json:"-"`              // true after first setup
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// Client represents the core client data returned by the main API
type Client struct {
	CPF        string `json:"cpf"`
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	InternalID string `json:"internal_id"` // Used to query other APIs
}

// Atendimento represents a support ticket
type Atendimento struct {
	ID          string    `json:"id"`
	Protocol    string    `json:"protocol"`
	Status      string    `json:"status"`
	Subject     string    `json:"subject"`
	CreatedAt   time.Time `json:"created_at"`
	OSStatus    string    `json:"os_status,omitempty"` // "Agendada", "Não Agendada", "Sem O.S."
	OSTecnico   string    `json:"os_tecnico,omitempty"`
	OSData      string    `json:"os_data,omitempty"`
}

// Fatura represents an invoice
type Fatura struct {
	ID            string    `json:"id"`
	Amount        float64   `json:"amount"`
	DueDate       string    `json:"due_date"`
	Status        string    `json:"status"` // PAID, PENDING, OVERDUE
	Barcode       string    `json:"barcode"`
}

// Financeiro represents financial status
type Financeiro struct {
	CreditScore int     `json:"credit_score"`
	TotalDebt   float64 `json:"total_debt"`
	IsDefaulter bool    `json:"is_defaulter"`
}

// Conexao represents a client's connection
type Conexao struct {
	CodConexao     int    `json:"codconexao"`
	Username       string `json:"username"`
	MacAddress     string `json:"mac_address"`
	Bloqueada      string `json:"bloqueada"`
	MotivoBloqueio string `json:"motivo_bloqueio"`
	Endereco       string `json:"endereco"`
	// Session data from WSMKConsultaConexaoAutenticada
	Down        string `json:"down,omitempty"`
	Up          string `json:"up,omitempty"`
	Tempo       string `json:"tempo,omitempty"`
	NAS         string `json:"nas,omitempty"`
	IP          string `json:"ip,omitempty"`
}

// Equipamento represents a client's inventory equipment (Router/ONT)
type Equipamento struct {
	Codigo           int    `json:"codigo"`
	DescricaoProduto string `json:"descricao_produto"`
	Status           string `json:"status"`
	Tipo             string `json:"tipo"`
	InSerial         string `json:"in_serial"`
}

// ClientAggregatedData represents the final centralized JSON
type ClientAggregatedData struct {
	Client       Client        `json:"client"`
	Atendimentos []Atendimento `json:"atendimentos"`
	Faturas      []Fatura      `json:"faturas"`
	Financeiro   Financeiro    `json:"financeiro"`
	Conexoes     []Conexao     `json:"conexoes"`
	Equipamentos []Equipamento `json:"equipamentos"`
}
