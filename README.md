# PastelZap API

API REST em Go e PostgreSQL para categorias, produtos, pedidos e fila da cozinha.

## Rodar localmente

Requer Go 1.27 e Docker. Copie `.env.example` para `.env`, gere `API_TOKEN` com `openssl rand -hex 32` e execute `docker compose up --build`.

Sem Docker, inicie um PostgreSQL, exporte `DATABASE_URL` e `API_TOKEN`, então rode `go run ./cmd/api`.

Variáveis: `PORT` (8080), `DATABASE_URL` (obrigatória), `API_TOKEN` (obrigatória, mínimo 32 caracteres), `DB_MAX_CONNS` (4) e `LOG_LEVEL` (info).

Execute `go test ./...`; os testes de integração usam `TEST_DATABASE_URL`. Exemplos de todas as chamadas estão em [requests.http](requests.http).

## Railway

Adicione PostgreSQL, publique este repositório e configure `DATABASE_URL=${{Postgres.DATABASE_URL}}` e um `API_TOKEN` novo. Gere domínio público em Networking. Para trocar o token, atualize a variável, publique novamente e gere novo APK.
