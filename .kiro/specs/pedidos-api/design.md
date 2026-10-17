# Documento de Design — Pedidos API

> Documento autocontido para implementação. Quem implementar deve seguir **exatamente** os nomes de arquivos, tipos, rotas, campos JSON, códigos de erro e mensagens definidos aqui. O app Android (repositório separado) depende deste contrato.

## 1. Visão Geral

API REST em **Go**, usando apenas a biblioteca padrão para HTTP e JSON e o driver `pgx/v5` para PostgreSQL. Gera um único binário estático executado em imagem `distroless`.

### 1.1 Stack e versões (fixadas)

| Item | Versão | Observação |
|---|---|---|
| Go | 1.27 (`go 1.27` no `go.mod`; build com `golang:1.27-alpine`) | Roteamento por método/padrão do `net/http` (Go 1.22+), `log/slog` |
| `github.com/jackc/pgx/v5` | v5.11.0 | Única dependência externa; usar `pgxpool` |
| PostgreSQL | 16 ou superior (serviço Postgres do Railway) | Local: `postgres:17-alpine` |
| Imagem final | `gcr.io/distroless/static-debian12:nonroot` | Sem shell, usuário não-root |

Proibido adicionar frameworks web (gin, echo, chi, fiber), ORMs (gorm, ent), bibliotecas de migração ou de validação. O código é pequeno o suficiente para não precisar deles e cada dependência aumenta memória e imagem.

### 1.2 Por que Go

| Opção | RAM em repouso (aprox.) | Imagem | Observação |
|---|---|---|---|
| **Go (net/http + pgx)** | 10–20 MB | ~15 MB | Binário único, sem runtime externo |
| Node.js (Fastify) | 50–80 MB | ~150 MB | Runtime mais pesado |
| Python (FastAPI) | 60–100 MB | ~150 MB | Mais memória e dependências |
| Java/Kotlin (Spring) | 200+ MB | ~250 MB | Inadequado para o objetivo |

O Railway cobra por RAM/CPU consumidos, então Go é a opção mais econômica.

### 1.3 Decisões principais

- **Dinheiro em centavos** (`int64` no Go, `INTEGER`/`BIGINT` no banco). Nunca usar float.
- **Snapshot de preço**: `product_name` e `unit_price_cents` são copiados para o item no lançamento.
- **Exclusão lógica** de categorias e produtos (`active = false`).
- **Total desnormalizado** em `orders.total_cents`, recalculado em toda mutação de itens, na mesma transação.
- **Bloqueio pessimista** (`SELECT ... FOR UPDATE`) na linha do pedido em toda mutação.
- **Polling** pela cozinha (5 s) em vez de WebSocket/SSE: sem conexões persistentes, implementação trivial.
- **Token fixo** obrigatório em `Authorization: Bearer <token>`.

## 2. Arquitetura

```mermaid
flowchart LR
    A[App Android<br/>Garçom / Cozinha] -- "HTTPS + Authorization: Bearer token" --> B[Pedidos API<br/>binário Go]
    B -- "rede privada Railway<br/>postgres.railway.internal" --> C[(PostgreSQL<br/>Railway)]
    R[Healthcheck Railway] -- "GET /health (público)" --> B
```

Fluxo de uma requisição:

```mermaid
flowchart TD
    REQ[Requisição] --> REC[recoverer]
    REC --> LOG[requestLogger]
    LOG --> AUTH{auth: rota é GET /health?}
    AUTH -- sim --> MUX
    AUTH -- não, token válido --> LIM[bodyLimit 64 KB]
    AUTH -- não, token inválido --> E401[401 unauthorized]
    LIM --> MUX[http.ServeMux]
    MUX --> H[Handler: parse + validação]
    H --> S[Store: SQL pgx / transações]
    S --> DB[(PostgreSQL)]
```

Responsabilidades:

- **Handler** (`internal/httpapi`): lê path/query/corpo, valida formato e regras que não dependem do banco, chama o store, mapeia erros para HTTP.
- **Store** (`internal/store`): todo o SQL, transações, regras que dependem do banco (categoria existe, produto ativo, pedido aberto, item concluído).
- **Model** (`internal/model`): structs de domínio/JSON, structs de entrada, funções de validação e normalização puras (sem I/O).

## 3. Estrutura do Projeto

```
pedidos-api/
├── cmd/
│   └── api/
│       └── main.go                 # ponto de entrada
├── internal/
│   ├── config/
│   │   ├── config.go               # Load() a partir de env
│   │   └── config_test.go
│   ├── db/
│   │   ├── db.go                   # NewPool(), Migrate()
│   │   └── migrations/
│   │       └── 0001_init.sql
│   ├── model/
│   │   ├── category.go
│   │   ├── product.go
│   │   ├── order.go
│   │   ├── optional.go             # tipo Optional[T] para PATCH
│   │   ├── validate.go             # helpers de texto e FieldErrors
│   │   └── validate_test.go
│   ├── store/
│   │   ├── store.go                # Store struct, erros sentinela, helpers
│   │   ├── categories.go
│   │   ├── products.go
│   │   ├── orders.go
│   │   ├── kitchen.go
│   │   └── store_integration_test.go   # build tag integration
│   └── httpapi/
│       ├── server.go               # NewHandler(): mux + middlewares
│       ├── store_iface.go          # interface Store usada pelos handlers
│       ├── middleware.go
│       ├── respond.go              # writeJSON, writeError, decodeJSON, pathID
│       ├── errors.go               # catálogo de erros e mapeamento
│       ├── health.go
│       ├── categories.go
│       ├── products.go
│       ├── orders.go
│       ├── kitchen.go
│       ├── middleware_test.go
│       ├── fake_store_test.go
│       └── handlers_test.go
├── Dockerfile
├── .dockerignore
├── docker-compose.yml              # apenas para desenvolvimento local
├── railway.json
├── requests.http                   # exemplos (extensão REST Client)
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

Nome do módulo no `go.mod`: `pedidos-api`. Imports internos: `pedidos-api/internal/...`.

## 4. Configuração

### 4.1 Variáveis de ambiente

| Variável | Obrigatória | Padrão | Validação | Descrição |
|---|---|---|---|---|
| `PORT` | não | `8080` | inteiro 1–65535 | Porta HTTP. O Railway injeta automaticamente. |
| `DATABASE_URL` | **sim** | — | não vazia | URL do Postgres. No Railway: `${{Postgres.DATABASE_URL}}` (rede privada). |
| `API_TOKEN` | **sim** | — | ≥ 32 caracteres | Token aceito em `Authorization: Bearer`. |
| `DB_MAX_CONNS` | não | `4` | inteiro 1–20 | Máximo de conexões do pool. |
| `LOG_LEVEL` | não | `info` | `debug`, `info`, `warn`, `error` | Nível do log. |

### 4.2 `internal/config/config.go`

```go
package config

type Config struct {
    Port        int
    DatabaseURL string
    APIToken    string
    DBMaxConns  int32
    LogLevel    slog.Level
}

// Load lê as variáveis via getenv (injetável para testes) e retorna erro
// descritivo para cada problema. Nunca inclui o valor do token na mensagem.
func Load(getenv func(string) string) (Config, error)
```

Mensagens de erro (exatas):

- `DATABASE_URL é obrigatória`
- `API_TOKEN é obrigatória`
- `API_TOKEN deve ter pelo menos 32 caracteres`
- `PORT inválida`
- `DB_MAX_CONNS deve ser um inteiro entre 1 e 20`
- `LOG_LEVEL deve ser debug, info, warn ou error`

`Load` deve fazer `strings.TrimSpace` em todos os valores. Se houver mais de um erro, juntar com `errors.Join`.

### 4.3 `.env.example`

```dotenv
PORT=8080
DATABASE_URL=postgres://pedidos:pedidos@localhost:5432/pedidos?sslmode=disable
# Gere com: openssl rand -hex 32
API_TOKEN=troque-por-um-token-aleatorio-com-64-caracteres-hexadecimais-000
DB_MAX_CONNS=4
LOG_LEVEL=info
```

## 5. Inicialização (`cmd/api/main.go`)

Sequência:

1. Criar logger `slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))` e definir como padrão (`slog.SetDefault`). Antes de carregar a config, usar nível `info`.
2. `cfg, err := config.Load(os.Getenv)`; em erro: `slog.Error("configuração inválida", "error", err)` e `os.Exit(1)`.
3. `ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)`.
4. `pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)`; em erro, log e `os.Exit(1)`.
5. `db.Migrate(ctx, pool)`; em erro, log e `os.Exit(1)`.
6. `st := store.New(pool)`; `handler := httpapi.NewHandler(st, cfg.APIToken, slog.Default())`.
7. Criar o servidor:

```go
srv := &http.Server{
    Addr:              ":" + strconv.Itoa(cfg.Port),
    Handler:           handler,
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      15 * time.Second,
    IdleTimeout:       60 * time.Second,
    MaxHeaderBytes:    16 << 10,
}
```

8. Iniciar `srv.ListenAndServe()` em goroutine; logar `servidor iniciado` com a porta. Erro diferente de `http.ErrServerClosed` → log e `os.Exit(1)`.
9. Aguardar `<-ctx.Done()`; logar `encerrando`; `srv.Shutdown` com contexto de 10 s; `pool.Close()`; sair com 0.

Endereço `":"+porta` escuta em todas as interfaces (IPv4 e IPv6), compatível com a rede do Railway.

## 6. Banco de Dados

### 6.1 Pool (`internal/db/db.go`)

```go
func NewPool(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error)
```

- `pgxpool.ParseConfig(url)`; definir `MaxConns = maxConns`, `MinConns = 0`, `MaxConnIdleTime = 5 * time.Minute`, `MaxConnLifetime = 30 * time.Minute`, `HealthCheckPeriod = time.Minute`.
- `ConnConfig.RuntimeParams["timezone"] = "UTC"` e `ConnConfig.RuntimeParams["application_name"] = "pedidos-api"`.
- Após criar, `pool.Ping` com timeout de 5 s. Se falhar, fechar o pool e retornar o erro.

### 6.2 Migrações (`Migrate`)

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, pool *pgxpool.Pool) error
```

Algoritmo:

1. `conn, _ := pool.Acquire(ctx)`; `defer conn.Release()` (o advisory lock é por sessão, por isso uma conexão dedicada).
2. `SELECT pg_advisory_lock(7262701)`; `defer SELECT pg_advisory_unlock(7262701)`.
3. `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`.
4. Ler os nomes já aplicados.
5. Listar `migrations/*.sql` com `fs.Glob`, ordenar por nome (`sort.Strings`). A versão é o nome do arquivo sem `.sql` (ex.: `0001_init`).
6. Para cada arquivo não aplicado: iniciar transação, `Exec` do conteúdo inteiro, `INSERT INTO schema_migrations(version) VALUES ($1)`, commit. Logar `migração aplicada` com a versão.
7. Erro em qualquer passo: rollback e retornar `fmt.Errorf("migração %s: %w", version, err)`.

Observação: para executar vários comandos SQL num único `Exec`, usar `conn.Exec(ctx, sql)` **sem argumentos** (protocolo simples), o que o pgx permite.

### 6.3 Schema (`internal/db/migrations/0001_init.sql`)

```sql
CREATE TABLE categories (
  id          BIGSERIAL PRIMARY KEY,
  name        VARCHAR(60)  NOT NULL,
  active      BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- Nome único apenas entre categorias ativas, sem diferenciar maiúsculas/minúsculas.
CREATE UNIQUE INDEX ux_categories_name_active ON categories (lower(name)) WHERE active;

CREATE TABLE products (
  id           BIGSERIAL PRIMARY KEY,
  name         VARCHAR(100) NOT NULL,
  category_id  BIGINT       NOT NULL REFERENCES categories(id),
  price_cents  INTEGER      NOT NULL CHECK (price_cents BETWEEN 0 AND 99999999),
  active       BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX ix_products_category_active ON products (category_id) WHERE active;

CREATE TABLE orders (
  id             BIGSERIAL PRIMARY KEY,
  customer_name  VARCHAR(80),
  table_label    VARCHAR(20),
  status         VARCHAR(10) NOT NULL DEFAULT 'open'
                 CHECK (status IN ('open', 'closed', 'canceled')),
  total_cents    BIGINT      NOT NULL DEFAULT 0 CHECK (total_cents >= 0),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at      TIMESTAMPTZ,
  CHECK ((status = 'open') = (closed_at IS NULL))
);
CREATE INDEX ix_orders_open_created ON orders (created_at) WHERE status = 'open';
CREATE INDEX ix_orders_status_closed ON orders (status, closed_at DESC);

CREATE TABLE order_items (
  id                BIGSERIAL PRIMARY KEY,
  order_id          BIGINT       NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_id        BIGINT       NOT NULL REFERENCES products(id),
  product_name      VARCHAR(100) NOT NULL,
  quantity          INTEGER      NOT NULL CHECK (quantity BETWEEN 1 AND 999),
  unit_price_cents  INTEGER      NOT NULL CHECK (unit_price_cents >= 0),
  total_cents       BIGINT       NOT NULL CHECK (total_cents >= 0),
  notes             VARCHAR(200),
  status            VARCHAR(10)  NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'done')),
  done_at           TIMESTAMPTZ,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CHECK ((status = 'done') = (done_at IS NOT NULL))
);
CREATE INDEX ix_order_items_order ON order_items (order_id, created_at, id);
```

Notas:

- `total_cents` é `BIGINT` porque `999 × 99.999.999` excede o limite de `INTEGER`.
- Não há triggers: `updated_at` é definido explicitamente (`updated_at = now()`) em cada `UPDATE`.
- `now()` retorna o mesmo instante dentro de uma transação, o que é desejado.

```mermaid
erDiagram
    categories ||--o{ products : "possui"
    products ||--o{ order_items : "lançado em"
    orders ||--o{ order_items : "contém"
    categories { bigint id PK
                 varchar name
                 bool active }
    products { bigint id PK
               varchar name
               bigint category_id FK
               int price_cents
               bool active }
    orders { bigint id PK
             varchar customer_name
             varchar table_label
             varchar status
             bigint total_cents
             timestamptz closed_at }
    order_items { bigint id PK
                  bigint order_id FK
                  bigint product_id FK
                  varchar product_name
                  int quantity
                  int unit_price_cents
                  bigint total_cents
                  varchar notes
                  varchar status
                  timestamptz done_at }
```

## 7. Modelos (`internal/model`)

Todos os `time.Time` retornados pelo store devem estar em UTC (`.UTC()` após o scan). `encoding/json` serializa `time.Time` em RFC 3339 com `Z`.

### 7.1 Saída (JSON)

```go
type Category struct {
    ID            int64     `json:"id"`
    Name          string    `json:"name"`
    ProductsCount int       `json:"products_count"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type Product struct {
    ID           int64     `json:"id"`
    Name         string    `json:"name"`
    CategoryID   int64     `json:"category_id"`
    CategoryName string    `json:"category_name"`
    PriceCents   int64     `json:"price_cents"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

const (
    OrderOpen     = "open"
    OrderClosed   = "closed"
    OrderCanceled = "canceled"
    ItemPending   = "pending"
    ItemDone      = "done"
)

type OrderItem struct {
    ID             int64      `json:"id"`
    ProductID      int64      `json:"product_id"`
    ProductName    string     `json:"product_name"`
    Quantity       int        `json:"quantity"`
    UnitPriceCents int64      `json:"unit_price_cents"`
    TotalCents     int64      `json:"total_cents"`
    Notes          *string    `json:"notes"`
    Status         string     `json:"status"`
    DoneAt         *time.Time `json:"done_at"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
}

// Usado na listagem (sem itens).
type OrderSummary struct {
    ID           int64      `json:"id"`
    CustomerName *string    `json:"customer_name"`
    TableLabel   *string    `json:"table_label"`
    Status       string     `json:"status"`
    TotalCents   int64      `json:"total_cents"`
    ItemsCount   int        `json:"items_count"`
    PendingCount int        `json:"pending_count"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
    ClosedAt     *time.Time `json:"closed_at"`
}

// Detalhe: resumo + itens. Items nunca nil (usar make(..., 0)).
type Order struct {
    OrderSummary
    Items []OrderItem `json:"items"`
}

type KitchenOrder struct {
    ID              int64       `json:"id"`
    CustomerName    *string     `json:"customer_name"`
    TableLabel      *string     `json:"table_label"`
    CreatedAt       time.Time   `json:"created_at"`
    OldestPendingAt *time.Time  `json:"oldest_pending_at"`
    Items           []OrderItem `json:"items"`
}

type KitchenResponse struct {
    ServerTime time.Time      `json:"server_time"`
    Orders     []KitchenOrder `json:"orders"`
}
```

O embed de `OrderSummary` em `Order` faz o JSON sair "achatado" (campos do resumo + `items`), que é o formato desejado.

### 7.2 Entrada (JSON)

```go
type CategoryInput struct {
    Name *string `json:"name"`
}

type ProductInput struct {
    Name       *string `json:"name"`
    CategoryID *int64  `json:"category_id"`
    PriceCents *int64  `json:"price_cents"`
}

type NewItemInput struct {
    ProductID *int64  `json:"product_id"`
    Quantity  *int    `json:"quantity"`
    Notes     *string `json:"notes"`
}

type CreateOrderInput struct {
    CustomerName *string        `json:"customer_name"`
    TableLabel   *string        `json:"table_label"`
    Items        []NewItemInput `json:"items"`
}

type AddItemsInput struct {
    Items []NewItemInput `json:"items"`
}

type UpdateOrderInput struct {
    CustomerName Optional[string] `json:"customer_name"`
    TableLabel   Optional[string] `json:"table_label"`
}

type UpdateItemInput struct {
    Quantity Optional[int]    `json:"quantity"`
    Notes    Optional[string] `json:"notes"`
}

type ItemStatusInput struct {
    Status *string `json:"status"`
}
```

Ponteiros em campos obrigatórios permitem distinguir "ausente" de "zero" na validação.

### 7.3 `Optional[T]` (`optional.go`) — semântica de PATCH

```go
// Optional distingue: campo ausente (Set=false), null (Set=true, Null=true)
// e valor (Set=true, Null=false, Value=v).
type Optional[T any] struct {
    Set   bool
    Null  bool
    Value T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
    o.Set = true
    if string(b) == "null" {
        o.Null = true
        return nil
    }
    return json.Unmarshal(b, &o.Value)
}
```

O `encoding/json` só chama `UnmarshalJSON` para campos presentes no corpo; campos ausentes ficam com `Set=false`.

### 7.4 Valores normalizados (saída da validação, entrada do store)

```go
type CategoryData struct{ Name string }

type ProductData struct {
    Name       string
    CategoryID int64
    PriceCents int64
}

type NewItem struct {
    ProductID int64
    Quantity  int
    Notes     *string // nil = sem observação
}

type CreateOrderData struct {
    CustomerName *string
    TableLabel   *string
    Items        []NewItem
}

// Para PATCH: Set indica se o campo deve ser alterado; Value nil = limpar.
type OptText struct {
    Set   bool
    Value *string
}

type UpdateOrderData struct {
    CustomerName OptText
    TableLabel   OptText
}

type UpdateItemData struct {
    Quantity *int  // nil = não alterar
    Notes    OptText
}

type OrderFilter struct {
    Status     string     // open | closed | canceled
    Limit      int        // 1..200
    ClosedFrom *time.Time // somente para closed/canceled
}
```

### 7.5 Validação (`validate.go`)

```go
// FieldErrors mapeia caminho do campo -> mensagem. Ex.: "items[0].quantity".
type FieldErrors map[string]string

func (f FieldErrors) Add(field, msg string) // só adiciona se o campo ainda não tiver erro
func (f FieldErrors) Any() bool

// requiredText: trim; vazio ou ausente -> "obrigatório"; > max runas -> "deve ter no máximo N caracteres".
func requiredText(f FieldErrors, field string, v *string, max int) string
// optionalText: nil ou vazio após trim -> nil; > max runas -> erro.
func optionalText(f FieldErrors, field string, v *string, max int) *string

func ValidateCategory(in CategoryInput) (CategoryData, FieldErrors)
func ValidateProduct(in ProductInput) (ProductData, FieldErrors)
func ValidateCreateOrder(in CreateOrderInput) (CreateOrderData, FieldErrors)
func ValidateAddItems(in AddItemsInput) ([]NewItem, FieldErrors)
func ValidateUpdateOrder(in UpdateOrderInput) (UpdateOrderData, FieldErrors)
func ValidateUpdateItem(in UpdateItemInput) (UpdateItemData, FieldErrors)
func ValidateItemStatus(in ItemStatusInput) (string, FieldErrors)
func ValidateOrderFilter(q url.Values) (OrderFilter, FieldErrors)
```

Contagem de caracteres: `utf8.RuneCountInString(strings.TrimSpace(s))`.

#### Regras e mensagens (exatas)

| Campo | Regra | Mensagem em `fields` |
|---|---|---|
| `name` (categoria) | obrigatório, 1–60 | `obrigatório` / `deve ter no máximo 60 caracteres` |
| `name` (produto) | obrigatório, 1–100 | `obrigatório` / `deve ter no máximo 100 caracteres` |
| `category_id` (corpo) | obrigatório, > 0 | `obrigatório` / `valor inválido` |
| `category_id` (store) | categoria ativa existe | `categoria não encontrada` |
| `price_cents` | obrigatório, 0–99999999 | `obrigatório` / `deve estar entre 0 e 99999999` |
| `customer_name` | opcional, ≤ 80 | `deve ter no máximo 80 caracteres` |
| `table_label` | opcional, ≤ 20 | `deve ter no máximo 20 caracteres` |
| `items` (criar pedido) | opcional, ≤ 50 | `máximo de 50 itens por requisição` |
| `items` (adicionar) | obrigatório, 1–50 | `informe ao menos um item` / `máximo de 50 itens por requisição` |
| `items[i].product_id` | obrigatório, > 0 | `obrigatório` / `valor inválido` |
| `items[i].product_id` (store) | produto ativo existe | `produto não encontrado ou inativo` |
| `items[i].quantity` | obrigatório, 1–999 | `obrigatório` / `deve estar entre 1 e 999` |
| `items[i].notes` | opcional, ≤ 200 | `deve ter no máximo 200 caracteres` |
| `quantity` (PATCH item) | se presente: não nulo, 1–999 | `não pode ser nulo` / `deve estar entre 1 e 999` |
| `notes` (PATCH item) | se presente: null/vazio limpa, ≤ 200 | `deve ter no máximo 200 caracteres` |
| corpo PATCH item vazio | ao menos `quantity` ou `notes` | chave `_` : `informe ao menos um campo` |
| corpo PATCH pedido vazio | ao menos um campo | chave `_` : `informe ao menos um campo` |
| `status` (item) | `pending` ou `done` | `obrigatório` / `deve ser pending ou done` |
| `status` (query) | `open`, `closed`, `canceled`; padrão `open` | `deve ser open, closed ou canceled` |
| `limit` (query) | inteiro 1–200; padrão 50 | `deve ser um inteiro entre 1 e 200` |
| `closed_from` (query) | RFC 3339 (`time.Parse(time.RFC3339, v)`) | `data inválida, use RFC 3339` |
| `category_id` (query de produtos) | inteiro > 0 | `valor inválido` |
| `include_done` (query) | `true`, `false` ou ausente | `deve ser true ou false` |

Quando a lista `items` tiver mais de 50 elementos, retornar apenas o erro de `items` (não validar cada item).

## 8. Camada HTTP (`internal/httpapi`)

### 8.1 Montagem (`server.go`)

```go
func NewHandler(st Store, apiToken string, log *slog.Logger) http.Handler {
    mux := http.NewServeMux()
    h := &handlers{st: st, log: log}

    mux.HandleFunc("GET /health", h.health)

    mux.HandleFunc("GET /categories", h.listCategories)
    mux.HandleFunc("POST /categories", h.createCategory)
    mux.HandleFunc("PUT /categories/{id}", h.updateCategory)
    mux.HandleFunc("DELETE /categories/{id}", h.deleteCategory)

    mux.HandleFunc("GET /products", h.listProducts)
    mux.HandleFunc("POST /products", h.createProduct)
    mux.HandleFunc("GET /products/{id}", h.getProduct)
    mux.HandleFunc("PUT /products/{id}", h.updateProduct)
    mux.HandleFunc("DELETE /products/{id}", h.deleteProduct)

    mux.HandleFunc("GET /orders", h.listOrders)
    mux.HandleFunc("POST /orders", h.createOrder)
    mux.HandleFunc("GET /orders/{id}", h.getOrder)
    mux.HandleFunc("PATCH /orders/{id}", h.updateOrder)
    mux.HandleFunc("POST /orders/{id}/items", h.addItems)
    mux.HandleFunc("PATCH /orders/{id}/items/{itemId}", h.updateItem)
    mux.HandleFunc("DELETE /orders/{id}/items/{itemId}", h.deleteItem)
    mux.HandleFunc("PATCH /orders/{id}/items/{itemId}/status", h.setItemStatus)
    mux.HandleFunc("POST /orders/{id}/close", h.closeOrder)
    mux.HandleFunc("POST /orders/{id}/cancel", h.cancelOrder)

    mux.HandleFunc("GET /kitchen/orders", h.kitchenOrders)

    // Qualquer outra rota/método: 404 JSON.
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        writeError(w, errNotFound)
    })

    var handler http.Handler = mux
    handler = bodyLimit(handler, 64<<10)
    handler = requireToken(handler, apiToken)
    handler = requestLogger(handler, log)
    handler = recoverer(handler, log)
    return handler
}
```

Ordem efetiva de execução: `recoverer` → `requestLogger` → `requireToken` → `bodyLimit` → `mux`. O padrão `"/"` captura métodos não registrados (ex.: `DELETE /orders`) e retorna `404 not_found` em JSON — comportamento aceito.

### 8.2 Middlewares (`middleware.go`)

**recoverer**: `defer` com `recover()`; em panic, logar `slog.Error("panic", "error", rec, "stack", string(debug.Stack()))` e, se a resposta ainda não foi escrita, `writeError(w, errInternal)`.

**requestLogger**: envolve o `ResponseWriter` num `statusRecorder` (guarda status, padrão 200). Após a execução, loga:

```go
level := slog.LevelInfo
if r.URL.Path == "/health" { level = slog.LevelDebug }
log.Log(r.Context(), level, "request",
    "method", r.Method, "path", r.URL.Path, "status", rec.status,
    "duration_ms", time.Since(start).Milliseconds())
```

Nunca logar cabeçalhos, query string completa ou corpo.

**requireToken**:

```go
func requireToken(next http.Handler, token string) http.Handler {
    expected := []byte(token)
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet && r.URL.Path == "/health" {
            next.ServeHTTP(w, r)
            return
        }
        got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
        if !ok || subtle.ConstantTimeCompare([]byte(got), expected) != 1 {
            writeError(w, errUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

`ConstantTimeCompare` retorna 0 imediatamente para tamanhos diferentes; isso revela apenas o tamanho do token, o que é aceitável.

**bodyLimit**: `r.Body = http.MaxBytesReader(w, r.Body, limit)`.

### 8.3 Respostas (`respond.go`)

```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.Header().Set("Cache-Control", "no-store")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func writeNoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// decodeJSON decodifica exatamente um objeto JSON em dst.
// Retorna *apiError pronto para writeError.
func decodeJSON(r *http.Request, dst any) *apiError {
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(dst); err != nil {
        var mbe *http.MaxBytesError
        if errors.As(err, &mbe) { return errPayloadTooLarge }
        return errInvalidJSON
    }
    if err := dec.Decode(&struct{}{}); err != io.EOF {
        var mbe *http.MaxBytesError
        if errors.As(err, &mbe) { return errPayloadTooLarge }
        return errInvalidJSON
    }
    return nil
}

// pathID lê r.PathValue(name); retorna (id, true) se for inteiro > 0.
func pathID(r *http.Request, name string) (int64, bool)
```

Corpo vazio resulta em `io.EOF` no primeiro `Decode` → `invalid_json`. Para `POST /orders/{id}/close` e `/cancel` o corpo é ignorado (não chamar `decodeJSON`). Se `pathID` falhar, responder `404 not_found`.

### 8.4 Erros (`errors.go`)

```go
type apiError struct {
    Status  int
    Code    string
    Message string
    Fields  map[string]string
}

// JSON: {"error":{"code":"...","message":"...","fields":{...}}}  (fields omitido se vazio)
func writeError(w http.ResponseWriter, e *apiError)

func validationError(f model.FieldErrors) *apiError
```

Catálogo (status, código e mensagem exatos):

| Variável | HTTP | `code` | `message` |
|---|---|---|---|
| `errInvalidJSON` | 400 | `invalid_json` | `JSON inválido` |
| `errUnauthorized` | 401 | `unauthorized` | `não autorizado` |
| `errNotFound` | 404 | `not_found` | `recurso não encontrado` |
| `errCategoryConflict` | 409 | `conflict` | `já existe uma categoria com este nome` |
| `errCategoryInUse` | 409 | `category_in_use` | `categoria possui produtos ativos` |
| `errOrderNotOpen` | 409 | `order_not_open` | `pedido não está aberto` |
| `errOrderEmpty` | 409 | `order_empty` | `pedido não possui itens` |
| `errItemDone` | 409 | `item_already_done` | `item já foi concluído pela cozinha` |
| `errPayloadTooLarge` | 413 | `payload_too_large` | `corpo da requisição excede 64 KB` |
| `validationError(...)` | 422 | `validation_error` | `dados inválidos` |
| `errInternal` | 500 | `internal_error` | `erro interno` |
| `errDBUnavailable` | 503 | `db_unavailable` | `banco de dados indisponível` |

Mapeamento de erros do store (`fromStore(err error) *apiError`):

| Erro do store | apiError |
|---|---|
| `store.ErrNotFound` | `errNotFound` |
| `store.ErrCategoryConflict` | `errCategoryConflict` |
| `store.ErrCategoryInUse` | `errCategoryInUse` |
| `store.ErrOrderNotOpen` | `errOrderNotOpen` |
| `store.ErrOrderEmpty` | `errOrderEmpty` |
| `store.ErrItemDone` | `errItemDone` |
| `*store.ValidationError` (`errors.As`) | `validationError(v.Fields)` |
| qualquer outro | logar `slog.Error("erro no store", "error", err, "path", r.URL.Path)` e `errInternal` |

### 8.5 Interface do store usada pelos handlers (`store_iface.go`)

```go
type Store interface {
    Ping(ctx context.Context) error

    ListCategories(ctx context.Context) ([]model.Category, error)
    CreateCategory(ctx context.Context, d model.CategoryData) (model.Category, error)
    UpdateCategory(ctx context.Context, id int64, d model.CategoryData) (model.Category, error)
    DeleteCategory(ctx context.Context, id int64) error

    ListProducts(ctx context.Context, categoryID *int64) ([]model.Product, error)
    GetProduct(ctx context.Context, id int64) (model.Product, error)
    CreateProduct(ctx context.Context, d model.ProductData) (model.Product, error)
    UpdateProduct(ctx context.Context, id int64, d model.ProductData) (model.Product, error)
    DeleteProduct(ctx context.Context, id int64) error

    ListOrders(ctx context.Context, f model.OrderFilter) ([]model.OrderSummary, error)
    GetOrder(ctx context.Context, id int64) (model.Order, error)
    CreateOrder(ctx context.Context, d model.CreateOrderData) (model.Order, error)
    UpdateOrder(ctx context.Context, id int64, d model.UpdateOrderData) (model.Order, error)
    AddItems(ctx context.Context, id int64, items []model.NewItem) (model.Order, error)
    UpdateItem(ctx context.Context, id, itemID int64, d model.UpdateItemData) (model.Order, error)
    DeleteItem(ctx context.Context, id, itemID int64) (model.Order, error)
    SetItemStatus(ctx context.Context, id, itemID int64, status string) (model.OrderItem, error)
    CloseOrder(ctx context.Context, id int64) (model.Order, error)
    CancelOrder(ctx context.Context, id int64) (model.Order, error)

    KitchenOrders(ctx context.Context, includeDone bool) (model.KitchenResponse, error)
}
```

`*store.Store` deve satisfazer essa interface (adicionar `var _ httpapi.Store = (*store.Store)(nil)` em um teste ou no `main.go`).

### 8.6 Padrão de handler

```go
func (h *handlers) createProduct(w http.ResponseWriter, r *http.Request) {
    var in model.ProductInput
    if e := decodeJSON(r, &in); e != nil { writeError(w, e); return }
    d, fe := model.ValidateProduct(in)
    if fe.Any() { writeError(w, validationError(fe)); return }
    p, err := h.st.CreateProduct(r.Context(), d)
    if err != nil { writeError(w, h.fromStore(r, err)); return }
    writeJSON(w, http.StatusCreated, p)
}
```

Todos os handlers seguem: parse de path → parse de query/corpo → validação → store → resposta.

## 9. Contrato dos Endpoints

Todas as rotas exceto `GET /health` exigem `Authorization: Bearer <API_TOKEN>`. Em todas, falha de token → `401`. Todas as respostas têm `Content-Type: application/json; charset=utf-8`, exceto `204`.

### 9.1 `GET /health` (público)

- Executa `st.Ping` com `context.WithTimeout(r.Context(), 2*time.Second)`.
- `200` → `{"status":"ok"}` · `503` → `db_unavailable`.

### 9.2 Categorias

**`GET /categories`** → `200`

```json
[
  { "id": 1, "name": "Bebidas", "products_count": 4,
    "created_at": "2026-10-13T20:00:00Z", "updated_at": "2026-10-13T20:00:00Z" }
]
```

```sql
SELECT c.id, c.name, COUNT(p.id) AS products_count, c.created_at, c.updated_at
FROM categories c
LEFT JOIN products p ON p.category_id = c.id AND p.active
WHERE c.active
GROUP BY c.id
ORDER BY lower(c.name), c.id;
```

**`POST /categories`** — corpo `{"name":"Bebidas"}` → `201` com o objeto (`products_count: 0`). Erros: `400`, `409 conflict`, `422`.

```sql
INSERT INTO categories (name) VALUES ($1)
RETURNING id, name, created_at, updated_at;
```

Violação de unique (`pgconn.PgError.Code == "23505"`) → `store.ErrCategoryConflict`.

**`PUT /categories/{id}`** — corpo `{"name":"Bebidas geladas"}` → `200` com o objeto (incluindo `products_count` atual). Erros: `400`, `404`, `409 conflict`, `422`.

```sql
UPDATE categories SET name = $2, updated_at = now()
WHERE id = $1 AND active
RETURNING id, name, created_at, updated_at;
-- 0 linhas -> ErrNotFound; 23505 -> ErrCategoryConflict.
-- Em seguida: SELECT COUNT(*) FROM products WHERE category_id = $1 AND active;
```

**`DELETE /categories/{id}`** → `204`. Erros: `404`, `409 category_in_use`. Em transação:

```sql
SELECT id FROM categories WHERE id = $1 AND active FOR UPDATE;          -- 0 linhas -> ErrNotFound
SELECT EXISTS (SELECT 1 FROM products WHERE category_id = $1 AND active); -- true -> ErrCategoryInUse
UPDATE categories SET active = FALSE, updated_at = now() WHERE id = $1;
```

### 9.3 Produtos

Objeto produto:

```json
{ "id": 3, "name": "X-Burguer", "category_id": 2, "category_name": "Lanches",
  "price_cents": 2000, "created_at": "2026-10-13T20:00:00Z", "updated_at": "2026-10-13T20:00:00Z" }
```

**`GET /products`** e **`GET /products?category_id=2`** → `200` com lista. Erro: `422` se `category_id` inválido.

```sql
SELECT p.id, p.name, p.category_id, c.name, p.price_cents, p.created_at, p.updated_at
FROM products p
JOIN categories c ON c.id = p.category_id
WHERE p.active AND ($1::bigint IS NULL OR p.category_id = $1)
ORDER BY lower(c.name), c.id, lower(p.name), p.id;
```

**`GET /products/{id}`** → `200` · `404`. Mesmo SELECT com `WHERE p.id = $1 AND p.active`.

**`POST /products`** — corpo:

```json
{ "name": "X-Burguer", "category_id": 2, "price_cents": 2000 }
```

→ `201` com o objeto. Erros: `400`, `422`. Em transação:

```sql
SELECT name FROM categories WHERE id = $1 AND active FOR SHARE;  -- 0 linhas -> ValidationError{category_id: "categoria não encontrada"}
INSERT INTO products (name, category_id, price_cents) VALUES ($1, $2, $3)
RETURNING id, created_at, updated_at;
```

`FOR SHARE` impede que a categoria seja inativada em paralelo enquanto o produto é criado.

**`PUT /products/{id}`** — mesmo corpo do POST (todos os campos obrigatórios) → `200`. Erros: `400`, `404`, `422`. Em transação: valida categoria (`FOR SHARE`) e:

```sql
UPDATE products SET name = $2, category_id = $3, price_cents = $4, updated_at = now()
WHERE id = $1 AND active
RETURNING id, created_at, updated_at;   -- 0 linhas -> ErrNotFound
```

Ordem de verificação no PUT: primeiro se o produto existe (`404`), depois a categoria (`422`).

**`DELETE /products/{id}`** → `204` · `404`.

```sql
UPDATE products SET active = FALSE, updated_at = now() WHERE id = $1 AND active;  -- 0 linhas -> ErrNotFound
```

### 9.4 Pedidos

Objeto pedido completo (resposta de `GET /orders/{id}` e de todas as mutações de pedido):

```json
{
  "id": 42,
  "customer_name": "João",
  "table_label": "5",
  "status": "open",
  "total_cents": 5300,
  "items_count": 2,
  "pending_count": 1,
  "created_at": "2026-10-13T22:10:00Z",
  "updated_at": "2026-10-13T22:15:00Z",
  "closed_at": null,
  "items": [
    {
      "id": 101, "product_id": 3, "product_name": "X-Burguer",
      "quantity": 2, "unit_price_cents": 2000, "total_cents": 4000,
      "notes": "sem cebola", "status": "done",
      "done_at": "2026-10-13T22:14:00Z",
      "created_at": "2026-10-13T22:10:00Z", "updated_at": "2026-10-13T22:14:00Z"
    },
    {
      "id": 102, "product_id": 7, "product_name": "Refrigerante lata",
      "quantity": 1, "unit_price_cents": 1300, "total_cents": 1300,
      "notes": null, "status": "pending", "done_at": null,
      "created_at": "2026-10-13T22:10:00Z", "updated_at": "2026-10-13T22:10:00Z"
    }
  ]
}
```

#### Carregar pedido completo (função interna `loadOrder(ctx, q querier, id)`)

`querier` é uma interface satisfeita por `*pgxpool.Pool` e `pgx.Tx` (`Query`, `QueryRow`, `Exec`).

```sql
SELECT id, customer_name, table_label, status, total_cents, created_at, updated_at, closed_at
FROM orders WHERE id = $1;                    -- 0 linhas -> ErrNotFound

SELECT id, product_id, product_name, quantity, unit_price_cents, total_cents, notes,
       status, done_at, created_at, updated_at
FROM order_items WHERE order_id = $1
ORDER BY created_at, id;
```

`items_count` = `len(items)`; `pending_count` = itens com `status = 'pending'` (calculados no Go).

#### Mutação com bloqueio (função interna `mutateOpenOrder`)

```go
func (s *Store) mutateOpenOrder(ctx context.Context, id int64,
    fn func(ctx context.Context, tx pgx.Tx) error) (model.Order, error) {
    // 1. tx := pool.Begin(ctx); defer tx.Rollback(ctx)
    // 2. SELECT status FROM orders WHERE id = $1 FOR UPDATE
    //      0 linhas -> ErrNotFound; status != 'open' -> ErrOrderNotOpen
    // 3. fn(ctx, tx)  (erros retornam direto)
    // 4. UPDATE orders SET total_cents = COALESCE((SELECT SUM(total_cents)
    //        FROM order_items WHERE order_id = $1), 0), updated_at = now() WHERE id = $1
    // 5. order := loadOrder(ctx, tx, id)
    // 6. tx.Commit(ctx); return order
}
```

Usada por: `UpdateOrder`, `AddItems`, `UpdateItem`, `DeleteItem`, e (com variação) `SetItemStatus`.

#### Inserção de itens (função interna `insertItems(ctx, tx, orderID, items)`)

1. Coletar os `ProductID` distintos.
2. ```sql
   SELECT id, name, price_cents FROM products WHERE id = ANY($1) AND active;
   ```
3. Para cada item `i` cujo produto não veio no resultado: `fields["items[i].product_id"] = "produto não encontrado ou inativo"`. Se houver algum, retornar `&ValidationError{Fields}` (a transação sofre rollback).
4. Inserir cada item usando `pgx.Batch` (uma ida ao banco):

```sql
INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price_cents, total_cents, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7);
```

com `total_cents = int64(quantity) * price_cents` calculado no Go.

#### Endpoints

**`GET /orders?status=open&limit=50&closed_from=2026-10-13T03:00:00Z`** → `200` com lista de `OrderSummary`. Erro: `422`.

Para `status = open`:

```sql
SELECT o.id, o.customer_name, o.table_label, o.status, o.total_cents,
       COUNT(i.id) AS items_count,
       COUNT(i.id) FILTER (WHERE i.status = 'pending') AS pending_count,
       o.created_at, o.updated_at, o.closed_at
FROM orders o
LEFT JOIN order_items i ON i.order_id = o.id
WHERE o.status = 'open'
GROUP BY o.id
ORDER BY o.created_at, o.id
LIMIT $1;
```

Para `closed`/`canceled`:

```sql
SELECT ... (mesmas colunas) ...
FROM orders o
LEFT JOIN order_items i ON i.order_id = o.id
WHERE o.status = $1 AND ($2::timestamptz IS NULL OR o.closed_at >= $2)
GROUP BY o.id
ORDER BY o.closed_at DESC, o.id DESC
LIMIT $3;
```

**`GET /orders/{id}`** → `200` pedido completo · `404`.

**`POST /orders`** — corpo:

```json
{
  "customer_name": "João",
  "table_label": "5",
  "items": [
    { "product_id": 3, "quantity": 2, "notes": "sem cebola" },
    { "product_id": 7, "quantity": 1 }
  ]
}
```

Todos os campos são opcionais (`{}` é válido). → `201` pedido completo. Erros: `400`, `422`. Em transação:

```sql
INSERT INTO orders (customer_name, table_label) VALUES ($1, $2) RETURNING id;
```

depois `insertItems`, recálculo do total (mesmo UPDATE do passo 4 de `mutateOpenOrder`), `loadOrder`, commit.

**`PATCH /orders/{id}`** — corpo com qualquer combinação de `customer_name`, `table_label` (valor, `null` ou `""`):

```json
{ "table_label": "7" }
```

→ `200` pedido completo. Erros: `400`, `404`, `409 order_not_open`, `422`. Dentro de `mutateOpenOrder`, alterar apenas os campos `Set`, usando uma única query fixa (sem montar SQL dinâmico):

```sql
UPDATE orders SET
  customer_name = CASE WHEN $2::boolean THEN $3::text ELSE customer_name END,
  table_label   = CASE WHEN $4::boolean THEN $5::text ELSE table_label END
WHERE id = $1;
```

Os casts explícitos (`::boolean`, `::text`, `::int`) evitam o erro do Postgres "inconsistent types deduced for parameter" e devem ser mantidos em todas as queries que reutilizam o mesmo parâmetro.

**`POST /orders/{id}/items`** — corpo `{"items":[{"product_id":3,"quantity":1}]}` → `200` pedido completo. Erros: `400`, `404`, `409 order_not_open`, `422`. Dentro de `mutateOpenOrder` chamar `insertItems`.

**`PATCH /orders/{id}/items/{itemId}`** — corpo com `quantity` e/ou `notes`:

```json
{ "quantity": 3, "notes": null }
```

→ `200` pedido completo. Erros: `400`, `404`, `409 order_not_open`, `409 item_already_done`, `422`. Dentro de `mutateOpenOrder`:

```sql
SELECT status, unit_price_cents FROM order_items WHERE id = $1 AND order_id = $2 FOR UPDATE;
-- 0 linhas -> ErrNotFound; status = 'done' -> ErrItemDone
UPDATE order_items SET
  quantity    = COALESCE($3::int, quantity),
  total_cents = COALESCE($3::int, quantity)::bigint * unit_price_cents,
  notes       = CASE WHEN $4::boolean THEN $5::text ELSE notes END,
  updated_at  = now()
WHERE id = $1;
```

(`$3` é `*int` — `nil` mantém a quantidade; `$4` é `bool` "notes Set"; `$5` é `*string`.)

**`DELETE /orders/{id}/items/{itemId}`** → `200` pedido completo. Erros: `404`, `409 order_not_open`. Dentro de `mutateOpenOrder`:

```sql
DELETE FROM order_items WHERE id = $1 AND order_id = $2;   -- 0 linhas -> ErrNotFound
```

**`PATCH /orders/{id}/items/{itemId}/status`** — corpo `{"status":"done"}` ou `{"status":"pending"}` → `200` com **apenas o item** atualizado:

```json
{ "id": 102, "product_id": 7, "product_name": "Refrigerante lata", "quantity": 1,
  "unit_price_cents": 1300, "total_cents": 1300, "notes": null, "status": "done",
  "done_at": "2026-10-13T22:16:00Z", "created_at": "2026-10-13T22:10:00Z",
  "updated_at": "2026-10-13T22:16:00Z" }
```

Erros: `400`, `404`, `409 order_not_open`, `422`. Em transação: `SELECT status FROM orders WHERE id = $1 FOR UPDATE` (404/409), depois:

```sql
UPDATE order_items SET
  status     = $3::text,
  done_at    = CASE WHEN $3::text = 'done' THEN COALESCE(done_at, now()) ELSE NULL END,
  updated_at = CASE WHEN status = $3::text THEN updated_at ELSE now() END
WHERE id = $1 AND order_id = $2
RETURNING id, product_id, product_name, quantity, unit_price_cents, total_cents, notes,
          status, done_at, created_at, updated_at;          -- 0 linhas -> ErrNotFound
UPDATE orders SET updated_at = now() WHERE id = $2;
```

Repetir o mesmo status é idempotente (mantém `done_at` e `updated_at` do item).

**`POST /orders/{id}/close`** → `200` pedido completo. Erros: `404`, `409 order_not_open`, `409 order_empty`. Em transação:

```sql
SELECT status FROM orders WHERE id = $1 FOR UPDATE;            -- 404 / 409 order_not_open
SELECT EXISTS (SELECT 1 FROM order_items WHERE order_id = $1); -- false -> ErrOrderEmpty
UPDATE orders SET status = 'closed', closed_at = now(), updated_at = now() WHERE id = $1;
```

**`POST /orders/{id}/cancel`** → `200` pedido completo. Erros: `404`, `409 order_not_open`. Igual ao fechamento sem a verificação de itens e com `status = 'canceled'`.

### 9.5 Cozinha

**`GET /kitchen/orders?include_done=false`** → `200`:

```json
{
  "server_time": "2026-10-13T22:20:00Z",
  "orders": [
    {
      "id": 42,
      "customer_name": "João",
      "table_label": "5",
      "created_at": "2026-10-13T22:10:00Z",
      "oldest_pending_at": "2026-10-13T22:10:00Z",
      "items": [ { "...": "objeto OrderItem completo, como em 9.4" } ]
    }
  ]
}
```

`server_time` = `time.Now().UTC()` no momento da resposta; o app usa esse valor para calcular o tempo de espera sem depender do relógio do aparelho. `oldest_pending_at` é `null` quando todos os itens estão concluídos (só aparece com `include_done=true`).

Consulta única (linhas achatadas, agrupadas no Go por `order.id` consecutivo):

```sql
WITH k AS (
  SELECT o.id,
         MIN(i.created_at) FILTER (WHERE i.status = 'pending') AS oldest_pending_at
  FROM orders o
  JOIN order_items i ON i.order_id = o.id
  WHERE o.status = 'open'
  GROUP BY o.id
  HAVING $1::boolean OR bool_or(i.status = 'pending')
)
SELECT o.id, o.customer_name, o.table_label, o.created_at, k.oldest_pending_at,
       i.id, i.product_id, i.product_name, i.quantity, i.unit_price_cents, i.total_cents,
       i.notes, i.status, i.done_at, i.created_at, i.updated_at
FROM k
JOIN orders o      ON o.id = k.id
JOIN order_items i ON i.order_id = o.id
ORDER BY k.oldest_pending_at ASC NULLS LAST, o.created_at, o.id, i.created_at, i.id;
```

`orders` vazio → `[]`.

## 10. Store (`internal/store`)

```go
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store

var (
    ErrNotFound         = errors.New("not found")
    ErrCategoryConflict = errors.New("category name conflict")
    ErrCategoryInUse    = errors.New("category in use")
    ErrOrderNotOpen     = errors.New("order not open")
    ErrOrderEmpty       = errors.New("order empty")
    ErrItemDone         = errors.New("item already done")
)

type ValidationError struct{ Fields map[string]string }
func (e *ValidationError) Error() string { return "validation error" }
```

Regras de implementação:

- Toda transação: `tx, err := s.pool.Begin(ctx)`; `defer tx.Rollback(ctx)` (no-op após commit).
- `pgx.ErrNoRows` → `ErrNotFound` (usar `errors.Is`).
- Unique violation: `var pgErr *pgconn.PgError; errors.As(err, &pgErr) && pgErr.Code == "23505"`.
- Converter todos os `time.Time` lidos para UTC.
- `Ping(ctx)` → `s.pool.Ping(ctx)`.
- Nunca concatenar valores do usuário em SQL; sempre parâmetros `$n`.

## 11. Deploy

### 11.1 `Dockerfile`

```dockerfile
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
ENV GOMEMLIMIT=24MiB
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
```

`GOMEMLIMIT` faz o GC trabalhar para manter o heap abaixo de ~24 MiB.

### 11.2 `.dockerignore`

```
.git
.kiro
.env
*.md
requests.http
docker-compose.yml
```

### 11.3 `railway.json`

```json
{
  "$schema": "https://railway.com/railway.schema.json",
  "build": { "builder": "DOCKERFILE", "dockerfilePath": "Dockerfile" },
  "deploy": {
    "healthcheckPath": "/health",
    "healthcheckTimeout": 60,
    "restartPolicyType": "ON_FAILURE",
    "restartPolicyMaxRetries": 5
  }
}
```

### 11.4 `docker-compose.yml` (somente desenvolvimento)

```yaml
services:
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: pedidos
      POSTGRES_PASSWORD: pedidos
      POSTGRES_DB: pedidos
    ports: ["5432:5432"]
    volumes: [pgdata:/var/lib/postgresql/data]
  api:
    build: .
    environment:
      DATABASE_URL: postgres://pedidos:pedidos@db:5432/pedidos?sslmode=disable
      API_TOKEN: ${API_TOKEN:?defina API_TOKEN no .env}
      LOG_LEVEL: debug
    ports: ["8080:8080"]
    depends_on: [db]
volumes:
  pgdata:
```

### 11.5 Passo a passo no Railway (vai no README)

1. Criar projeto no Railway e adicionar o serviço **PostgreSQL**.
2. Adicionar serviço a partir do repositório GitHub `pedidos-api` (o Railway detecta `railway.json`/`Dockerfile`).
3. No serviço da API, em *Variables*: `DATABASE_URL = ${{Postgres.DATABASE_URL}}` (URL privada) e `API_TOKEN = <saída de openssl rand -hex 32>`.
4. Em *Settings > Networking*, gerar domínio público (HTTPS). Essa URL vai no build do app.
5. Opcional: ativar **Serverless** (suspensão por inatividade) no serviço da API para não consumir recursos fora do horário; a primeira requisição após a suspensão demora alguns segundos.
6. Para trocar o token: alterar `API_TOKEN`, redeploy, gerar novo APK com o novo token.

## 12. Segurança

- Sem login de usuários (requisito). A API só aceita requisições com o token `API_TOKEN`, embutido no APK no build.
- A API não inicia sem token ou com token curto (< 32 caracteres).
- Token gerado com `openssl rand -hex 32` (64 caracteres hexadecimais, 256 bits): tentativa e erro é inviável.
- HTTPS fornecido pelo domínio do Railway; o token não trafega em texto aberto.
- Token e cabeçalhos nunca são logados; mensagens de 401 não indicam o motivo.
- Todas as queries são parametrizadas; corpo limitado a 64 KB; campos desconhecidos rejeitados.
- **Limitação conhecida**: o token fica dentro do APK; alguém com o arquivo e conhecimento técnico consegue extraí-lo. Para o cenário (APK instalado apenas nos aparelhos da casa, não publicado em loja) o nível de proteção é adequado. Se houver suspeita de vazamento, trocar o token (seção 11.5, passo 6).

## 13. Estratégia de Testes

### 13.1 Unitários (sem banco) — `go test ./...`

- `config_test.go`: ausência de `DATABASE_URL`/`API_TOKEN`, token com 31 e 32 caracteres, `PORT` inválida, padrões aplicados, mensagem de erro não contém o token.
- `validate_test.go`: tabela de casos para cada função `Validate*`, incluindo trim, contagem de runas com acentos ("Açaí"), limites (0, 1, máx., máx.+1), `null` vs ausente no `Optional`, 51 itens.
- `middleware_test.go` (`httptest`): sem cabeçalho → 401; `Basic xxx` → 401; `Bearer` errado → 401; `Bearer` correto → passa; `GET /health` sem token → passa; `POST /health` sem token → 401; panic → 500 JSON.
- `handlers_test.go` com `fakeStore` (implementa `Store` em memória ou com funções configuráveis): status e corpo de cada rota, mapeamento de cada erro sentinela, JSON malformado, campo desconhecido, corpo > 64 KB → 413, id inválido no path → 404, rota desconhecida → 404 JSON.

### 13.2 Integração (com Postgres) — `go test -tags integration ./internal/store/...`

Executados somente se `TEST_DATABASE_URL` estiver definida (senão `t.Skip`). Cada teste roda `Migrate` e limpa as tabelas (`TRUNCATE order_items, orders, products, categories RESTART IDENTITY CASCADE`). Cenários:

- Categoria: nome duplicado (case-insensitive) → conflito; excluir com produto ativo → em uso; recriar nome após inativar.
- Produto: categoria inativa → validação; alterar preço não muda itens já lançados.
- Pedido: criação com itens calcula totais; produto inativo faz rollback total; adicionar/alterar/remover recalcula total; item `done` não pode ser alterado; pedido fechado rejeita alterações; fechar pedido vazio → `ErrOrderEmpty`.
- Concorrência: 10 goroutines adicionando itens ao mesmo pedido → total final igual à soma dos itens.
- Cozinha: ordenação por pendente mais antigo; `include_done`; pedidos fechados não aparecem.

### 13.3 Verificação manual

`requests.http` com variáveis `@baseUrl` e `@token` e uma requisição para cada rota, na ordem: health → criar categoria → criar produto → criar pedido → cozinha → marcar item → fechar.

## 14. README.md (conteúdo obrigatório)

1. Descrição curta e stack.
2. Pré-requisitos (Go 1.27, Docker).
3. Rodar local: `cp .env.example .env`, gerar token, `docker compose up --build`.
4. Rodar sem Docker: subir Postgres, exportar variáveis, `go run ./cmd/api`.
5. Testes: `go test ./...` e integração com `TEST_DATABASE_URL`.
6. Tabela de variáveis de ambiente (seção 4.1).
7. Deploy no Railway (seção 11.5).
8. Resumo dos endpoints com link para `requests.http`.
9. Como trocar o token.
