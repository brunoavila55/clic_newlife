<h1 align="center">
  <img src="https://img.shields.io/badge/Clic-NewLife-00C9A7?style=for-the-badge&labelColor=0A1628" alt="Clic NewLife" />
</h1>

<p align="center">
  Dashboard de monitoramento e centralização de dados de clientes integrado com o ERP <strong>MK Solutions</strong>.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/versão-0.1-00C9A7?style=flat-square" />
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go" />
  <img src="https://img.shields.io/badge/Fiber-v2-00C9A7?style=flat-square" />
  <img src="https://img.shields.io/badge/SQLite-embedded-003B57?style=flat-square&logo=sqlite" />
  <img src="https://img.shields.io/badge/HTMX-dynamic-3366CC?style=flat-square" />
</p>

---

## 📋 Visão Geral

O **Clic NewLife** é uma aplicação web monolítica em Go que consolida informações de clientes do ERP MK Solutions em um único painel. Com ele é possível:

- 🔍 **Consultar clientes** por CPF ou CNPJ
- 📡 **Monitorar conexões** (IP, tráfego, uptime, histórico de quedas)
- 📄 **Visualizar faturas vencidas** em atraso
- 🛠️ **Acompanhar atendimentos** e Ordens de Serviço
- 📦 **Inventariar equipamentos** em comodato (roteadores/ONTs)
- 👥 **Gerenciar usuários** do sistema (admin e usuários comuns)

## 🏗️ Arquitetura

```
clic_newlife/
├── backend/
│   ├── cmd/api/main.go          # Ponto de entrada da aplicação
│   ├── internal/
│   │   ├── application/         # Casos de uso (orquestração)
│   │   ├── config/              # Leitura de variáveis de ambiente
│   │   ├── domain/              # Modelos de domínio (structs)
│   │   ├── infrastructure/      # Banco de dados e integrações externas
│   │   └── presentation/        # Handlers HTTP e middlewares
│   ├── views/                   # Templates HTML (SSR)
│   │   └── layouts/main.html    # Layout base com navbar
│   ├── data/                    # Banco SQLite (gerado automaticamente)
│   ├── .env.example             # Exemplo de variáveis de ambiente
│   └── Dockerfile
├── docker-compose.yml
├── run_native.ps1               # Script de execução nativa (Windows)
└── README.md
```

**Stack:**
- **Backend:** Go + [Fiber v2](https://gofiber.io/) (framework web)
- **Banco de dados:** SQLite via [GORM](https://gorm.io/) (apenas para autenticação de usuários)
- **Templates:** HTML Server-Side Rendering com [HTMX](https://htmx.org/) para interações assíncronas
- **Autenticação:** JWT com cookies `HTTPOnly`

---

## 🚀 Instalação e Execução

### Pré-requisitos

- [Go 1.21+](https://go.dev/dl/) instalado
- Acesso à API do MK Solutions ERP (URL, token e senha)

### 1. Clone o repositório

```bash
git clone https://github.com/SEU_USUARIO/clic_newlife.git
cd clic_newlife
```

### 2. Configure as variáveis de ambiente

```bash
cd backend
cp .env.example .env
```

Abra o arquivo `backend/.env` e preencha com suas credenciais:

```env
PORT=8080

# Segredo JWT — use uma string longa e aleatória
JWT_SECRET=troque-por-uma-chave-secreta-longa-e-aleatoria

# URL base do seu servidor MK Solutions
MK_API_URL=http://SEU_IP_OU_HOST_MK:8080

# Token de autenticação da API MK Solutions
MK_AUTH_TOKEN=seu-token-mk-aqui

# Senha de autenticação da API MK Solutions
MK_AUTH_PASSWORD=sua-senha-mk-aqui
```

> ⚠️ **Nunca commite o arquivo `.env`!** Ele já está no `.gitignore`.

### 3. Baixe as dependências e execute

```bash
# Dentro da pasta backend/
go mod download
go run cmd/api/main.go
```

Acesse o dashboard em: **http://localhost:8080**

---

## 🔐 Primeiro Acesso

Ao iniciar pela primeira vez, um usuário administrador padrão é criado automaticamente:

| Campo  | Valor   |
|--------|---------|
| Usuário | `admin` |
| Senha  | `admin` |

> 🔑 **Importante:** No primeiro login, o sistema irá redirecionar automaticamente para a página de **configuração de senha**. Defina uma senha forte antes de usar o sistema em produção. Esse processo ocorre **apenas uma vez** — reinicializações subsequentes não pedem nova configuração.

---

## 👥 Sistema de Usuários

O sistema possui dois níveis de acesso:

| Perfil  | Permissões |
|---------|-----------|
| `admin` | Acesso completo, inclui painel de gerenciamento de usuários (`/admin/users`) |
| `user`  | Acesso ao dashboard de consulta de clientes apenas |

### Criar novos usuários

Acesse **http://localhost:8080/admin/users** com uma conta admin para criar, listar e remover usuários do sistema.

---

## 🐳 Execução com Docker (Produção)

```bash
# Na raiz do projeto
docker-compose up -d
```

O `docker-compose.yml` já configura as variáveis de ambiente básicas. Edite-o para ajustar o `JWT_SECRET` e as variáveis da API MK antes de subir em produção:

```yaml
environment:
  - JWT_SECRET=sua-chave-secreta-aqui
  - MK_API_URL=http://SEU_IP_MK:8080
  - MK_AUTH_TOKEN=seu-token-mk
  - MK_AUTH_PASSWORD=sua-senha-mk
```

---

## 🔒 Segurança

- **Dados de clientes não são armazenados.** O banco SQLite local contém apenas usuários do sistema (credenciais de acesso ao dashboard).
- Cada consulta busca os dados diretamente da API MK em tempo real.
- Senhas são armazenadas com hash **bcrypt**.
- Sessões usam **JWT** em cookies `HTTPOnly` (não acessíveis via JavaScript).

---

## 📡 Integrações com MK Solutions

As seguintes APIs são consumidas:

| API | Descrição |
|-----|-----------|
| `WSAutenticacao` | Autenticação e obtenção de token de sessão |
| `WSMKConsultaDoc` | Busca de cliente por CPF/CNPJ |
| `WSMKConexoesPorCliente` | Lista de conexões do cliente |
| `WSMKConsultaConexaoAutenticada` | Sessão Radius ativa (IP, tráfego, uptime) |
| `WSMKFaturasAbertas` | Faturas vencidas em atraso |
| `WSMKConsultaOrdemAgendamento` | Agendamentos de Ordens de Serviço |
| `WSMKConsultaProdutoEstoque` | Inventário de equipamentos em comodato |

---

## 📄 Licença

Projeto proprietário — uso interno. © 2025 Clic NewLife.
