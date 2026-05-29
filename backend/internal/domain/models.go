package domain

import (
	"time"

	"gorm.io/gorm"
)

// User represents a system user (for authentication)
type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Email     string         `gorm:"uniqueIndex;not null" json:"email"`
	Password  string         `gorm:"not null" json:"-"`
	Name      string         `json:"name"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
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
}

// Fatura represents an invoice
type Fatura struct {
	ID            string    `json:"id"`
	Amount        float64   `json:"amount"`
	DueDate       time.Time `json:"due_date"`
	Status        string    `json:"status"` // PAID, PENDING, OVERDUE
	Barcode       string    `json:"barcode"`
}

// Financeiro represents financial status
type Financeiro struct {
	CreditScore int     `json:"credit_score"`
	TotalDebt   float64 `json:"total_debt"`
	IsDefaulter bool    `json:"is_defaulter"`
}

// Contrato represents a client's contract
type Contrato struct {
	ID        string    `json:"id"`
	PlanName  string    `json:"plan_name"`
	Status    string    `json:"status"` // ACTIVE, CANCELLED
	StartDate time.Time `json:"start_date"`
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
	Down  string `json:"down,omitempty"`
	Up    string `json:"up,omitempty"`
	Tempo string `json:"tempo,omitempty"`
	NAS   string `json:"nas,omitempty"`
}

// ClientAggregatedData represents the final centralized JSON
type ClientAggregatedData struct {
	Client       Client        `json:"client"`
	Atendimentos []Atendimento `json:"atendimentos"`
	Faturas      []Fatura      `json:"faturas"`
	Financeiro   Financeiro    `json:"financeiro"`
	Contratos    []Contrato    `json:"contratos"`
	Conexoes     []Conexao     `json:"conexoes"`
}
