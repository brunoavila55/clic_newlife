# Contexto de Desenvolvimento: Clic NewLife (Dashboard de Clientes MK Solutions)

Este arquivo serve como um guia completo de arquitetura, integrações de APIs e decisões de design tomadas no projeto **Clic NewLife**, facilitando a continuidade do desenvolvimento em qualquer sessão futura.

---

## 1. Visão Geral da Aplicação

O **Clic NewLife** é um dashboard de monitoramento e centralização de dados de clientes integrado com os webservices da **MK Solutions ERP**.
* **Arquitetura**: Aplicação Go (Golang) monolítica utilizando o framework **Fiber** para rotas e renderização nativa de templates.
* **Banco de Dados**: SQLite local (`data/clic.db`) gerenciado via GORM, utilizado estritamente para controle de autenticação interna da plataforma.
* **Frontend**: Templates HTML renderizados no servidor em `views`, enriquecidos dinamicamente com **TailwindCSS** (design escuro e glassmorphism premium) e interações parciais assíncronas com **HTMX**.

---

## 2. Como Rodar e Testar

* **Pré-requisitos**: Go v1.26+ instalado localmente.
* **Passos para execução**:
  1. Acesse o diretório raiz (onde está o `go.mod` e o `.env`).
  2. Execute:
     ```powershell
     go run cmd/api/main.go
     ```
  3. Acesse o dashboard no navegador pelo endereço: [http://localhost:8080](http://localhost:8080)
* **Credenciais de Acesso Local**:
  * **Usuário**: `admin@admin.com`
  * **Senha**: `admin`

---

## 3. Mapeamento das Integrações de APIs (MK Solutions)

Toda a lógica de integração com as APIs da MK está localizada no arquivo [client_api.go](file:///c:/clic_newlife/internal/infrastructure/integration/client_api.go).

### A. Autenticação Principal (`WSAutenticacao.rule`)
* **Endpoint**: `/mk/WSAutenticacao.rule?sys=MK0&token={MK_AUTH_TOKEN}&cd_servico=9999`
* **Função**: Autentica o sistema e retorna um token de sessão dinâmico válido utilizado nas demais requisições.

### B. Consulta de Cliente por CPF/CNPJ (`WSMKConsultaDoc.rule`)
* **Endpoint**: `/mk/WSMKConsultaDoc.rule?sys=MK0&token={SessionToken}&doc={CPF_DIGITS}`
* **Função**: Localiza os dados cadastrais básicos do cliente e retorna o `CodigoPessoa` (mapeado como `internalID`), usado como chave primária para todas as consultas seguintes.

### C. Monitor de Conexões (Múltiplas Conexões & Radius)
O monitor foi totalmente remodelado e suporta cenários onde o cliente possui mais de uma conexão ativa.

1. **Obter Conexões Pessoais (`WSMKConexoesPorCliente.rule`)**:
   * **Endpoint**: `/mk/WSMKConexoesPorCliente.rule?sys=MK0&token={SessionToken}&cd_cliente={internalID}`
   * **Retorno**: Uma lista de todas as conexões cadastradas para o cliente (contendo `codconexao`, `username` que representa o login PPPoE, `mac_address`, etc.).

2. **Enriquecer com Conexão Autenticada (`WSMKConsultaConexaoAutenticada.rule`)**:
   * **Endpoint**: `/mk/WSMKConsultaConexaoAutenticada.rule?sys=MK0&token={SessionToken}&codconexao={codconexao}`
   * **Tratamento de Schema (Crítico)**:
     * *Quirk descoberto*: A API pode retornar os dados em dois schemas diferentes dependendo da versão do ERP.
     * **Formato A (Postman)**: Retorna uma lista de agendas sob a chave `"Agendas"`: `{"Agendas": [{...}]}`.
     * **Formato B (Produção Real)**: Retorna um objeto único sob a chave `"conexao"`: `{"conexao": {...}}`.
     * **Solução**: O struct de parsing `MKSessionResponse` mapeia ambos dinamicamente de forma extremamente robusta.
   * **Dados Extraídos**:
     * `down` (Download atual da sessão) e `up` (Upload atual)
     * `tempo` (Uptime formatado da sessão Radius)
     * `nas` (Concentrador/NAS - exibido sutilmente em texto opaco)
     * `framedip` (IP atribuído à conexão - **Destaque visual absoluto e principal no design**)
     * `dt_hr_parada` (Última queda da conexão) e `dt_hr_retorno` (Último retorno da conexão), usados para desenhar o histórico de quedas de cada link.

### D. Consulta de Faturas Vencidas (`WSMKFaturasAbertas.rule`)
* **Endpoint**: `/mk/WSMKFaturasAbertas.rule?sys=MK0&token={SessionToken}&dt_venc_inicio={Inicio}&dt_venc_fim={Fim}&cd_pessoa={internalID}`
* **Decisão de Design / Requisito do Usuário**:
  * O ERP já lança todas as faturas do contrato de forma programada em lote (aparecendo todas em aberto no futuro).
  * **Regra**: O usuário **não tem interesse** em visualizar faturas a vencer ou futuras. O dashboard deve monitorar e exibir **estritamente as faturas já vencidas em atraso**.
  * **Solução**: Filtramos a busca passando como parâmetro dinâmico obrigatório `dt_venc_inicio` a data `"01/01/2020"` e como `dt_venc_fim` a data de **ontem** (`time.Now().AddDate(0, 0, -1)`).
  * Qualquer fatura retornada nesse período do passado é considerada **Vencida** e ganha destaque em vermelho brilhante com o status `⚠ Vencida`. Se o período estiver limpo, exibe a mensagem de sucesso: *"Nenhuma fatura vencida"*.

### E. Agendamento de Ordens de Serviço (`WSMKConsultaOrdemAgendamento.rule`)
* **Endpoint**: `/mk/WSMKConsultaOrdemAgendamento.rule?sys=MK0&token={SessionToken}&cliente={internalID}&DataAbertura={formattedDate}`
* **Tratamento de Schema (Crítico)**:
  * *Quirk descoberto*: A chave JSON do array retornado por esta API possui um caractere de dois-pontos ao final: `"Agendas:": [...]`. O struct `MKOSResponse` mapeia essa chave exatamente usando a tag ``json:"Agendas:"``.
* **Integração no Fluxo**:
  * Para cada atendimento retornado na lista dos últimos 30 dias, extraímos o campo `DataAbertura` e fazemos a consulta de agendamento convertendo a data para o formato `DD/MM/AAAA`.
  * **Mapeamento de Estados da O.S.**:
    * Se a lista `"Agendas:"` vier vazia: Mapeia como **"Sem O.S."** (Não tem ordem de serviço cadastrada para este ticket).
    * Se a lista contiver elementos:
      * Se o campo `Data_agendamento` estiver preenchido: Mapeia como **"Agendada"** (O.S. confirmada com técnico associado e data agendada).
      * Se o campo `Data_agendamento` estiver vazio/nulo: Mapeia como **"Não Agendada"** (O.S. aberta no sistema mas aguardando agendamento físico).

### F. Inventário de Equipamentos em Estoque/Comodato (`WSMKConsultaProdutoEstoque.rule`)
* **Endpoint**: `/mk/WSMKConsultaProdutoEstoque.rule?sys=MK0&Token={SessionToken}&cd_setor={internalID}`
* **Regras de Negócio / Requisitos do Usuário**:
  * **Mapeamento de Chaves (Crítico)**: A API retorna os produtos sob a chave `"Códigos:"` ou `"Código:"` dependendo da versão do ERP. O código lida de forma dinâmica e transparente com ambas.
  * **Filtragem de Tipo/Modelo**: Filtra para exibir apenas os produtos cuja descrição (`descricao`) contenha `"roteador"` ou `"ont"` (case-insensitive).
  * **Destaque Visual**: O modelo do roteador/ONT (`descricao`) recebe **ênfase absoluta** no frontend com um degradê ciano vibrante.
  * **Mapeamento de Atributos**:
    * O campo `estoque_atual` (ex: 1.0) é exibido no status da UI como a quantidade (`Qtd: 1`).
    * O campo `descricao_setor` (ex: "Setor Gustavo") é exibido na UI no campo "Modo / Tipo".
    * O campo `controle_serial` (ex: "S") é exibido na UI no campo de Serial como `"Com Serial"` ou `"Sem Serial"`.
  * **Suporte Multi-itens**: Se o cliente/setor tiver múltiplos roteadores/ONTs em comodato, o painel lista todos em cards elegantes.

---

## 4. Arquivos-Chave do Projeto

1. **[models.go](file:///c:/clic_newlife/internal/domain/models.go)**:
   Contém as structs de domínio, incluindo `Conexao`, `Atendimento` e a nova struct `Equipamento` (com mapeamento de serial e descrição), vinculada ao `ClientAggregatedData`.
2. **[client_api.go](file:///c:/clic_newlife/internal/infrastructure/integration/client_api.go)**:
   Gerencia todas as requisições HTTP da API da MK. Adicionado o método `FetchEquipamentos` para consultar o inventário e aplicar os filtros de status e modelo de roteadores/ONTs.
3. **[dashboard.html](file:///c:/clic_newlife/views/dashboard.html)**:
   Template principal do painel. Reorganizado o layout da coluna 2 (`lg:col-span-5` com `space-y-4`) para acomodar o card do "Monitor de Conexões" e, logo abaixo, o novo card dedicado ao "Inventário de Equipamentos do Cliente" (exibindo número de série, modo/tipo em comodato e modelo destacado).
4. **[ui_handler.go](file:///c:/clic_newlife/internal/presentation/handler/ui_handler.go)**:
   Controlador que mapeia e injeta o array `Equipamentos` no template de visualização do dashboard.
5. **[fetch_client_data.go](file:///c:/clic_newlife/internal/application/usecase/fetch_client_data.go)**:
   Atualizado para incluir a busca concorrente de equipamentos no inventário do cliente em paralelo com as outras 5 requisições principais do sistema (subindo o contador `sync.WaitGroup` para 6).

---

## 5. Próximos Passos Recomendados
* **Otimização de Performance**: Atualmente as consultas parciais são feitas de forma assíncrona concorrente usando Go routines e `sync.WaitGroup` no usecase. A consulta de agendamento de O.S. dentro do loop de atendimentos funciona perfeitamente por abranger apenas os últimos 30 dias (geralmente poucos itens), mas se o volume de atendimentos crescer, pode ser interessante implementar paralelismo ou cache curto para as chamadas individuais de O.S.
