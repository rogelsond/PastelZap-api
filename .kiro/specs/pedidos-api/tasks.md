# Plano de Implementação — Pedidos API

> Execute as tarefas em ordem. Cada tarefa referencia as seções do `design.md` que contêm o detalhe necessário (estruturas, SQL, mensagens). Ao final de cada tarefa, `go build ./...` e `go vet ./...` devem passar. Tarefas marcadas com `*` são testes opcionais, mas recomendados.

- [ ] 1. Estruturar o projeto
  - Criar `go.mod` com `module pedidos-api` e `go 1.27`; adicionar `github.com/jackc/pgx/v5 v5.11.0` (`go get github.com/jackc/pgx/v5@v5.11.0`)
  - Criar todas as pastas da seção 3 do design
  - Criar `.gitignore` (`.env`, binários `/api`, `/bin/`, `*.exe`, `coverage.out`), `.env.example` (seção 4.3) e `.dockerignore` (seção 11.2)
  - _Requisitos: 9.3_

- [ ] 2. Configuração
  - [ ] 2.1 Implementar `internal/config/config.go` com `Config` e `Load(getenv)` conforme seção 4.2: trim, padrões, validações e mensagens exatas, `errors.Join` para múltiplos erros; `LogLevel` convertido para `slog.Level`
    - _Requisitos: 8.4, 9.3_
  - [ ]* 2.2 Escrever `config_test.go` (seção 13.1), garantindo que nenhuma mensagem de erro contenha o valor do token
    - _Requisitos: 8.4, 8.5_

- [ ] 3. Banco de dados
  - [ ] 3.1 Criar `internal/db/migrations/0001_init.sql` exatamente como a seção 6.3
    - _Requisitos: 1.1, 2.1, 3.1, 7.5_
  - [ ] 3.2 Implementar `NewPool` em `internal/db/db.go` (seção 6.1): parâmetros do pool, `timezone=UTC`, `application_name`, ping de 5 s
    - _Requisitos: 9.4, 9.5_
  - [ ] 3.3 Implementar `Migrate` (seção 6.2): `embed`, conexão dedicada, `pg_advisory_lock(7262701)`, tabela `schema_migrations`, aplicação ordenada em transação, logs
    - _Requisitos: 9.2_

- [ ] 4. Modelos e validação
  - [ ] 4.1 Criar `model/category.go`, `model/product.go`, `model/order.go` com as structs de saída (seção 7.1), entrada (7.2) e normalizadas (7.4), e as constantes de status
    - _Requisitos: 1.1, 2.1, 3.1, 5.7, 7.1_
  - [ ] 4.2 Criar `model/optional.go` com `Optional[T]` (seção 7.3)
    - _Requisitos: 4.2, 4.3, 4.7_
  - [ ] 4.3 Criar `model/validate.go`: `FieldErrors`, `requiredText`, `optionalText` e todas as funções `Validate*` seguindo a tabela de regras e mensagens da seção 7.5
    - _Requisitos: 1.2, 2.2, 2.6, 3.1, 3.9, 4.4, 5.6, 7.8_
  - [ ]* 4.4 Escrever `validate_test.go` com tabelas de casos (seção 13.1)
    - _Requisitos: 1.2, 2.2, 3.1, 3.9, 4.4_

- [ ] 5. Infraestrutura HTTP
  - [ ] 5.1 Criar `httpapi/errors.go`: struct `apiError`, catálogo completo com status/código/mensagem exatos e `validationError` (seção 8.4)
    - _Requisitos: 9.5, 9.6, 9.7_
  - [ ] 5.2 Criar `httpapi/respond.go`: `writeJSON` (com `Cache-Control: no-store`), `writeNoContent`, `writeError`, `decodeJSON` (campos desconhecidos, múltiplos valores, 413) e `pathID` (seção 8.3)
    - _Requisitos: 9.5, 9.6_
  - [ ] 5.3 Criar `httpapi/middleware.go`: `recoverer`, `requestLogger` com `statusRecorder` (sem logar cabeçalhos), `requireToken` (bypass só para `GET /health`, `ConstantTimeCompare`) e `bodyLimit` (seção 8.2)
    - _Requisitos: 8.1, 8.2, 8.3, 8.5, 9.7, 9.9_
  - [ ] 5.4 Criar `httpapi/store_iface.go` com a interface `Store` (seção 8.5) e a função `fromStore` de mapeamento de erros (seção 8.4)
  - [ ] 5.5 Criar `httpapi/server.go` com `NewHandler` registrando todas as rotas, o catch-all `"/"` → 404 JSON e a cadeia de middlewares na ordem da seção 8.1. Handlers ainda não implementados podem ficar como stubs que retornam 500
    - _Requisitos: 8.1_
  - [ ] 5.6 Criar `httpapi/health.go` (seção 9.1)
    - _Requisitos: 9.1_
  - [ ]* 5.7 Escrever `middleware_test.go` (seção 13.1)
    - _Requisitos: 8.1, 8.2, 9.7_

- [ ] 6. Store base e ponto de entrada
  - [ ] 6.1 Criar `store/store.go`: `Store`, `New`, `Ping`, erros sentinela, `ValidationError`, interface interna `querier`, helper `isUniqueViolation(err)` (seção 10)
  - [ ] 6.2 Implementar `cmd/api/main.go` conforme a seção 5 (logger, config, sinais, pool, migração, servidor com timeouts, shutdown de 10 s). Adicionar `var _ httpapi.Store = (*store.Store)(nil)`
    - _Requisitos: 8.4, 9.2, 9.4, 9.8_
  - [ ] 6.3 Verificar manualmente: subir Postgres com `docker compose up db`, rodar `go run ./cmd/api` sem `API_TOKEN` (deve sair com erro), com token curto (erro), com token válido (sobe e aplica migração); `GET /health` → 200; `GET /categories` sem token → 401
    - _Requisitos: 8.2, 8.4, 9.1, 9.2_

- [ ] 7. Categorias
  - [ ] 7.1 Implementar `store/categories.go`: `ListCategories`, `CreateCategory`, `UpdateCategory` (com contagem de produtos), `DeleteCategory` (transação com `FOR UPDATE`) usando o SQL da seção 9.2
    - _Requisitos: 1.1, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 1.10_
  - [ ] 7.2 Implementar `httpapi/categories.go`: os 4 handlers seguindo o padrão da seção 8.6 e os status `200/201/204`
    - _Requisitos: 1.1–1.10_
  - [ ]* 7.3 Adicionar casos de categorias em `handlers_test.go` com `fakeStore`
    - _Requisitos: 1.2, 1.3, 1.8, 1.10_

- [ ] 8. Produtos
  - [ ] 8.1 Implementar `store/products.go`: `ListProducts` (filtro opcional), `GetProduct`, `CreateProduct` e `UpdateProduct` (validação de categoria com `FOR SHARE`, retornando `ValidationError`), `DeleteProduct` (seção 9.3)
    - _Requisitos: 2.1, 2.3, 2.4, 2.5, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12_
  - [ ] 8.2 Implementar `httpapi/products.go`: 5 handlers; validar `category_id` da query (`422` se inválido)
    - _Requisitos: 2.1–2.12_
  - [ ]* 8.3 Adicionar casos de produtos em `handlers_test.go`
    - _Requisitos: 2.2, 2.6, 2.11_

- [ ] 9. Pedidos — leitura e criação
  - [ ] 9.1 Implementar em `store/orders.go` as funções internas `loadOrder`, `recalcTotal` e `insertItems` (busca em lote com `ANY($1)`, `ValidationError` por índice, `pgx.Batch`) (seção 9.4)
    - _Requisitos: 3.2, 3.3, 3.4, 3.5, 3.8_
  - [ ] 9.2 Implementar `ListOrders` (duas queries fixas: aberto / fechado-cancelado com `closed_from`) e `GetOrder`
    - _Requisitos: 5.1–5.9_
  - [ ] 9.3 Implementar `CreateOrder` em transação (insert do pedido, `insertItems`, recálculo, `loadOrder`, commit)
    - _Requisitos: 3.1–3.9_
  - [ ] 9.4 Implementar em `httpapi/orders.go` os handlers `listOrders`, `getOrder`, `createOrder`
    - _Requisitos: 3.1, 5.1–5.9_

- [ ] 10. Pedidos — manutenção, fechamento e cancelamento
  - [ ] 10.1 Implementar `mutateOpenOrder` (seção 9.4) com `SELECT ... FOR UPDATE`, verificação de status, recálculo do total e `updated_at`
    - _Requisitos: 4.8, 4.10, 4.11_
  - [ ] 10.2 Implementar `UpdateOrder`, `AddItems`, `UpdateItem` (bloqueio de item `done`) e `DeleteItem` usando `mutateOpenOrder` e as queries com casts explícitos
    - _Requisitos: 4.1–4.9_
  - [ ] 10.3 Implementar `CloseOrder` (verifica itens → `ErrOrderEmpty`) e `CancelOrder`
    - _Requisitos: 6.1–6.6_
  - [ ] 10.4 Implementar os handlers `updateOrder`, `addItems`, `updateItem`, `deleteItem`, `closeOrder`, `cancelOrder` (close/cancel ignoram o corpo)
    - _Requisitos: 4.1–4.9, 6.1–6.5_
  - [ ]* 10.5 Adicionar casos de pedidos em `handlers_test.go` (PATCH com campo ausente vs `null`, corpo vazio, mapeamento de cada erro 409)
    - _Requisitos: 4.3, 4.4, 4.5, 4.8, 6.3, 6.5_

- [ ] 11. Cozinha
  - [ ] 11.1 Implementar `store/kitchen.go`: `KitchenOrders` com a query única da seção 9.5, agrupando linhas consecutivas por pedido, `server_time` em UTC e listas nunca `nil`
    - _Requisitos: 7.1, 7.2, 7.3, 7.4, 7.9_
  - [ ] 11.2 Implementar `SetItemStatus` em `store/orders.go` (bloqueio do pedido, UPDATE idempotente, atualização de `orders.updated_at`)
    - _Requisitos: 7.5, 7.6, 7.7, 4.8_
  - [ ] 11.3 Implementar `httpapi/kitchen.go` (`kitchenOrders`, validando `include_done`) e o handler `setItemStatus`
    - _Requisitos: 7.1–7.8_
  - [ ]* 11.4 Adicionar casos de cozinha em `handlers_test.go`
    - _Requisitos: 7.3, 7.8_

- [ ] 12. Testes de integração
  - [ ]* 12.1 Criar `store/store_integration_test.go` com `//go:build integration`, pulando se `TEST_DATABASE_URL` não estiver definida, cobrindo todos os cenários da seção 13.2 (incluindo o teste de concorrência)
    - _Requisitos: 1.3, 1.8, 2.9, 3.4, 3.5, 4.5, 4.8, 4.10, 6.3, 7.2, 7.3, 7.4_

- [ ] 13. Empacotamento e documentação
  - [ ] 13.1 Criar `Dockerfile` (seção 11.1), `railway.json` (11.3) e `docker-compose.yml` (11.4)
    - _Requisitos: 9.1, 9.10_
  - [ ] 13.2 Criar `requests.http` (seção 13.3) com `@baseUrl = http://localhost:8080`, `@token = ...` e `Authorization: Bearer {{token}}` em todas as requisições exceto health
  - [ ] 13.3 Escrever `README.md` com todo o conteúdo da seção 14
    - _Requisitos: 8.6_
  - [ ] 13.4 Verificação final: `go vet ./...`, `go test ./...`, `docker compose up --build`, executar todo o `requests.http`, confirmar `401` sem token, conferir memória com `docker stats` (alvo < 30 MB) e tamanho da imagem (`docker images`, alvo < 25 MB)
    - _Requisitos: 8.2, 9.10_
