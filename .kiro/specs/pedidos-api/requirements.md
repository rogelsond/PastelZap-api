# Documento de Requisitos — Pedidos API

## Introdução

API REST para um sistema de pedidos de um estabelecimento amador (bar, lanchonete, evento). Ela é consumida exclusivamente por um aplicativo Android com duas visões:

- **Garçom**: cadastro de categorias e produtos, abertura e manutenção de pedidos, totalização e fechamento de conta.
- **Cozinha**: fila de pedidos com o detalhe do que cada mesa/cliente pediu e marcação dos itens preparados/enviados.

A API será hospedada no **Railway** junto com um banco **PostgreSQL** também no Railway. O objetivo principal de arquitetura é **consumo mínimo de memória e CPU**.

Não existe login de usuários. A segurança é feita por um **token fixo e obrigatório**: a API só atende requisições que tragam o token correto no cabeçalho `Authorization`. O mesmo token é embutido no app no momento do build, de modo que apenas o app consegue usar a API.

Este documento define **o que** a API deve fazer. O documento `design.md` define **como** (contratos JSON completos, SQL, estrutura de código) e o `tasks.md` define a ordem de implementação. Em caso de dúvida, o contrato do `design.md` é a referência para nomes de campos, códigos de status e mensagens.

## Glossário

| Termo | Definição |
|---|---|
| Categoria | Agrupamento de produtos (ex.: Bebidas, Lanches). Possui apenas nome. |
| Produto | Item vendável com nome, categoria e preço. |
| Pedido | Conta de uma mesa e/ou cliente. Contém itens. Mesa e cliente são opcionais. |
| Item do pedido | Produto lançado no pedido com quantidade, preço unitário (copiado do produto no momento do lançamento), total e observação opcional. |
| Status do pedido | `open` (aberto), `closed` (conta fechada), `canceled` (cancelado). |
| Status do item | `pending` (aguardando cozinha) ou `done` (preparado/enviado para a mesa). |
| Exclusão lógica | Marcar o registro como inativo (`active = false`) em vez de apagá-lo, preservando o histórico. |
| Token da API | Segredo compartilhado definido na variável de ambiente `API_TOKEN` e enviado pelo app no cabeçalho `Authorization: Bearer <token>`. |
| Centavos | Todos os valores monetários são inteiros em centavos (R$ 12,50 = `1250`). |

## Convenções gerais (valem para todos os requisitos)

1. Corpo de requisição e resposta em JSON UTF-8, com nomes de campo em `snake_case`.
2. Valores monetários: inteiros em centavos, campos com sufixo `_cents`.
3. Datas: strings RFC 3339 em UTC com sufixo `Z` (ex.: `2026-10-13T22:10:00.123456Z`; a fração de segundos é opcional).
4. Identificadores: inteiros positivos (64 bits).
5. Textos recebidos têm espaços nas pontas removidos (trim) antes da validação. Para campos opcionais, texto vazio após o trim é tratado como `null`.
6. Limites de tamanho de texto são contados em caracteres Unicode (não em bytes).
7. Listas na resposta nunca são `null`: quando vazias retornam `[]`.
8. Erros sempre no formato `{"error":{"code":"...","message":"...","fields":{...}}}` (campo `fields` presente apenas em erros de validação).

## Requisitos

### Requisito 1 — Cadastro de categorias

**User Story:** Como garçom, quero cadastrar, renomear, listar e excluir categorias, para organizar os produtos do cardápio.

#### Critérios de Aceitação

1. WHEN o app enviar `POST /categories` com `name` contendo de 1 a 60 caracteres após o trim THEN a API SHALL criar a categoria ativa e retornar `201` com o objeto da categoria (com `products_count = 0`).
2. IF `name` estiver ausente, não for string, estiver vazio após o trim ou tiver mais de 60 caracteres THEN a API SHALL retornar `422 validation_error` com a mensagem no campo `name`.
3. IF já existir outra categoria **ativa** com o mesmo nome, comparando sem diferenciar maiúsculas/minúsculas THEN a API SHALL retornar `409 conflict`.
4. WHEN o app enviar `GET /categories` THEN a API SHALL retornar todas as categorias ativas, ordenadas por nome (sem diferenciar maiúsculas/minúsculas), cada uma com `products_count` igual à quantidade de produtos ativos dela.
5. WHEN o app enviar `PUT /categories/{id}` com `name` válido THEN a API SHALL atualizar o nome, atualizar `updated_at` e retornar `200` com o objeto atualizado.
6. WHEN o app renomear uma categoria para o mesmo nome dela com outra capitalização (ex.: "bebidas" → "Bebidas") THEN a API SHALL aceitar a alteração.
7. WHEN o app enviar `DELETE /categories/{id}` e a categoria não possuir produtos ativos THEN a API SHALL inativá-la (exclusão lógica) e retornar `204` sem corpo.
8. IF a categoria a excluir possuir ao menos um produto ativo THEN a API SHALL retornar `409 category_in_use` sem alterar dados.
9. WHEN uma categoria for inativada THEN a API SHALL permitir criar uma nova categoria ativa com o mesmo nome.
10. IF o `{id}` não for um inteiro positivo, não existir ou pertencer a uma categoria inativa THEN a API SHALL retornar `404 not_found` em `PUT` e `DELETE`.

### Requisito 2 — Cadastro de produtos

**User Story:** Como garçom, quero cadastrar e manter produtos com nome, categoria e valor, para lançá-los nos pedidos.

#### Critérios de Aceitação

1. WHEN o app enviar `POST /products` com `name` (1 a 100 caracteres após o trim), `category_id` de uma categoria ativa e `price_cents` inteiro entre 0 e 99.999.999 THEN a API SHALL criar o produto ativo e retornar `201` com o objeto do produto, incluindo `category_name`.
2. IF algum campo for inválido THEN a API SHALL retornar `422 validation_error` informando **todos** os campos inválidos de uma vez em `fields`.
3. IF `category_id` não corresponder a uma categoria ativa THEN a API SHALL retornar `422 validation_error` com `fields.category_id = "categoria não encontrada"`.
4. WHEN o app enviar `GET /products` THEN a API SHALL retornar os produtos ativos, ordenados por nome da categoria e depois por nome do produto (sem diferenciar maiúsculas/minúsculas).
5. WHEN o app informar `?category_id={id}` em `GET /products` THEN a API SHALL retornar apenas os produtos ativos dessa categoria (lista vazia se a categoria não existir).
6. IF `category_id` na query não for um inteiro positivo THEN a API SHALL retornar `422 validation_error` com `fields.category_id = "valor inválido"`.
7. WHEN o app enviar `GET /products/{id}` de um produto ativo THEN a API SHALL retornar `200` com o produto.
8. WHEN o app enviar `PUT /products/{id}` com o corpo completo válido (mesmos campos e regras do `POST`) THEN a API SHALL substituir nome, categoria e preço e retornar `200`.
9. WHEN o preço de um produto for alterado THEN a API SHALL manter inalterados o preço e o total dos itens já lançados em pedidos.
10. WHEN o app enviar `DELETE /products/{id}` THEN a API SHALL inativar o produto (exclusão lógica) e retornar `204`, mantendo os itens de pedidos que o referenciam.
11. IF o produto não existir, estiver inativo ou o `{id}` for inválido THEN a API SHALL retornar `404 not_found` em `GET`, `PUT` e `DELETE` por id.
12. The API SHALL permitir produtos com nomes repetidos.

### Requisito 3 — Abertura de pedidos

**User Story:** Como garçom, quero abrir um pedido informando opcionalmente cliente e mesa e os produtos desejados, para enviá-lo à cozinha.

#### Critérios de Aceitação

1. WHEN o app enviar `POST /orders` com `customer_name` (opcional, até 80 caracteres), `table_label` (opcional, até 20 caracteres) e `items` (opcional, lista de 0 a 50 itens, cada um com `product_id`, `quantity` inteiro de 1 a 999 e `notes` opcional até 200 caracteres) THEN a API SHALL criar o pedido com status `open` e retornar `201` com o pedido completo (incluindo itens).
2. WHEN um item for lançado THEN a API SHALL copiar `product_name` e `unit_price_cents` do produto naquele momento e calcular `total_cents = quantity × unit_price_cents`.
3. WHEN itens forem lançados THEN a API SHALL criá-los com status `pending`.
4. WHEN um pedido for criado ou tiver itens alterados THEN a API SHALL recalcular `orders.total_cents` como a soma de `total_cents` de todos os itens, na mesma transação da alteração.
5. IF algum `product_id` não existir ou estiver inativo THEN a API SHALL retornar `422 validation_error` com `fields["items[i].product_id"] = "produto não encontrado ou inativo"` (onde `i` é o índice na lista) e SHALL NOT gravar nenhum dado.
6. WHERE nem cliente nem mesa forem informados THE API SHALL aceitar o pedido.
7. WHEN `items` estiver ausente ou vazio THEN a API SHALL criar o pedido aberto sem itens, com `total_cents = 0`.
8. WHEN a mesma `product_id` aparecer mais de uma vez em `items` THEN a API SHALL criar uma linha de item para cada ocorrência.
9. IF a lista tiver mais de 50 itens THEN a API SHALL retornar `422` com `fields.items = "máximo de 50 itens por requisição"`.

### Requisito 4 — Manutenção de pedidos abertos

**User Story:** Como garçom, quero adicionar, alterar e remover itens de um pedido aberto e corrigir mesa/cliente, para atender novos pedidos da mesa e corrigir erros.

#### Critérios de Aceitação

1. WHEN o app enviar `POST /orders/{id}/items` com `items` contendo de 1 a 50 itens válidos THEN a API SHALL adicioná-los ao pedido com status `pending`, recalcular o total e retornar `200` com o pedido completo.
2. WHEN o app enviar `PATCH /orders/{id}/items/{itemId}` com `quantity` e/ou `notes` THEN a API SHALL alterar apenas os campos presentes no corpo, recalcular `total_cents` do item e do pedido e retornar `200` com o pedido completo.
3. WHEN `notes` for enviado como `null` ou texto vazio THEN a API SHALL remover a observação do item.
4. IF o corpo do `PATCH` de item não contiver nenhum dos campos `quantity` e `notes`, ou `quantity` for `null` THEN a API SHALL retornar `422 validation_error`.
5. IF o item a alterar estiver com status `done` THEN a API SHALL retornar `409 item_already_done`.
6. WHEN o app enviar `DELETE /orders/{id}/items/{itemId}` THEN a API SHALL remover o item (independente do status dele), recalcular o total e retornar `200` com o pedido completo.
7. WHEN o app enviar `PATCH /orders/{id}` com `customer_name` e/ou `table_label` THEN a API SHALL alterar apenas os campos presentes; valor `null` ou texto vazio limpa o campo.
8. IF o pedido não estiver com status `open` THEN a API SHALL rejeitar qualquer alteração (dados, itens ou status de item) com `409 order_not_open`.
9. IF o item não existir ou não pertencer ao pedido informado THEN a API SHALL retornar `404 not_found`.
10. WHEN dois aparelhos alterarem o mesmo pedido ao mesmo tempo THEN a API SHALL serializar as alterações (bloqueio da linha do pedido) de forma que o total final seja sempre a soma correta dos itens.
11. WHEN qualquer alteração for feita em um pedido ou em seus itens THEN a API SHALL atualizar `orders.updated_at`.

### Requisito 5 — Consulta de pedidos

**User Story:** Como garçom, quero listar os pedidos e consultar o detalhe de cada um, para acompanhar as mesas.

#### Critérios de Aceitação

1. WHEN o app enviar `GET /orders` sem parâmetros THEN a API SHALL retornar os pedidos `open`, do mais antigo para o mais recente, limitados a 50.
2. WHEN o app informar `?status=open|closed|canceled` THEN a API SHALL filtrar por esse status.
3. WHEN o status filtrado for `closed` ou `canceled` THEN a API SHALL ordenar por `closed_at` do mais recente para o mais antigo.
4. WHEN o app informar `?limit=N` com N entre 1 e 200 THEN a API SHALL limitar a lista a N pedidos.
5. WHEN o app informar `?closed_from=<RFC 3339>` THEN a API SHALL retornar apenas pedidos com `closed_at` maior ou igual ao instante informado (usado pelo app para "Fechados hoje"); para `status=open` o parâmetro é ignorado.
6. IF `status`, `limit` ou `closed_from` forem inválidos THEN a API SHALL retornar `422 validation_error` com o campo correspondente.
7. WHEN a listagem for retornada THEN cada pedido SHALL conter `id`, `customer_name`, `table_label`, `status`, `total_cents`, `items_count` (quantidade de linhas de itens), `pending_count` (linhas com status `pending`), `created_at`, `updated_at` e `closed_at`, sem a lista de itens.
8. WHEN o app enviar `GET /orders/{id}` THEN a API SHALL retornar o pedido (de qualquer status) com os mesmos campos da listagem mais `items`, ordenados por data de lançamento e depois por id.
9. IF o pedido não existir THEN a API SHALL retornar `404 not_found`.

### Requisito 6 — Fechamento e cancelamento

**User Story:** Como garçom, quero fechar a conta com o total ou cancelar um pedido, para encerrar o atendimento.

#### Critérios de Aceitação

1. WHEN o app enviar `POST /orders/{id}/close` para um pedido `open` com ao menos um item THEN a API SHALL mudar o status para `closed`, registrar `closed_at` e retornar `200` com o pedido completo e o total final.
2. WHEN o pedido for fechado com itens ainda `pending` THEN a API SHALL permitir o fechamento (o aviso é responsabilidade do app).
3. IF o pedido a fechar não tiver itens THEN a API SHALL retornar `409 order_empty`.
4. WHEN o app enviar `POST /orders/{id}/cancel` para um pedido `open` THEN a API SHALL mudar o status para `canceled`, registrar `closed_at` e retornar `200` com o pedido completo.
5. IF o pedido já estiver `closed` ou `canceled` THEN a API SHALL retornar `409 order_not_open`.
6. The API SHALL NOT apagar fisicamente pedidos nem itens de pedidos fechados ou cancelados.

### Requisito 7 — Visão da cozinha

**User Story:** Como cozinheiro, quero ver o que cada mesa/cliente pediu, em ordem de chegada, e marcar o que já foi preparado/enviado, para organizar a produção.

#### Critérios de Aceitação

1. WHEN o app enviar `GET /kitchen/orders` THEN a API SHALL retornar `server_time` (hora atual do servidor em UTC) e `orders`: os pedidos `open` que possuam ao menos um item `pending`, cada um com todos os seus itens (pendentes e concluídos).
2. The API SHALL ordenar os pedidos da cozinha pelo item pendente mais antigo (`oldest_pending_at` crescente) e, em empate, pela criação do pedido; os itens de cada pedido SHALL ser ordenados por data de lançamento e id.
3. WHEN o app informar `?include_done=true` THEN a API SHALL incluir também os pedidos abertos cujos itens estejam todos `done`, posicionados após os que têm pendências.
4. The API SHALL NOT retornar na cozinha pedidos sem itens, nem pedidos `closed` ou `canceled`.
5. WHEN o app enviar `PATCH /orders/{id}/items/{itemId}/status` com `{"status":"done"}` THEN a API SHALL marcar o item como concluído, registrar `done_at` e retornar `200` com o item atualizado.
6. WHEN o app enviar `{"status":"done"}` para um item já `done` THEN a API SHALL retornar `200` mantendo o `done_at` original (operação idempotente).
7. WHEN o app enviar `{"status":"pending"}` THEN a API SHALL desfazer a marcação, limpar `done_at` e retornar `200`.
8. IF `status` for diferente de `pending` ou `done` THEN a API SHALL retornar `422` com `fields.status`.
9. The API SHALL atender `GET /kitchen/orders` com **uma única consulta** ao banco, para suportar polling a cada 5 segundos com custo baixo.

### Requisito 8 — Autenticação por token

**User Story:** Como responsável pelo sistema, quero que apenas o app consiga usar a API, verificando um token em cada requisição.

#### Critérios de Aceitação

1. The API SHALL exigir o cabeçalho `Authorization: Bearer <token>` em **todas** as rotas, incluindo rotas inexistentes, exceto `GET /health`.
2. IF o cabeçalho estiver ausente, não começar com `Bearer ` (com espaço) ou o token for diferente de `API_TOKEN` THEN a API SHALL retornar `401 unauthorized` com a mesma mensagem genérica em todos os casos, sem ler o corpo e sem acessar o banco.
3. The API SHALL comparar o token em tempo constante (`crypto/subtle.ConstantTimeCompare`).
4. IF a variável `API_TOKEN` não estiver definida ou tiver menos de 32 caracteres THEN a API SHALL encerrar na inicialização com código de saída diferente de zero e mensagem explicativa no log (sem exibir o valor).
5. The API SHALL NOT registrar o token nem o cabeçalho `Authorization` em logs.
6. WHEN o token precisar ser trocado THEN basta alterar `API_TOKEN` no Railway e gerar novo build do app, sem mudança de código.

### Requisito 9 — Saúde e operação

**User Story:** Como responsável pelo sistema, quero que a API seja barata de manter, robusta e fácil de publicar no Railway.

#### Critérios de Aceitação

1. WHEN alguém enviar `GET /health` THEN a API SHALL verificar a conexão com o banco (timeout de 2 s) e retornar `200 {"status":"ok"}`, ou `503 db_unavailable` se o banco não responder; a rota SHALL ser pública e não expor nenhum dado.
2. WHEN a API iniciar THEN ela SHALL aplicar automaticamente as migrações pendentes, cada uma em transação, protegidas por lock para não rodarem duas vezes em paralelo.
3. The API SHALL ler a configuração das variáveis de ambiente `PORT`, `DATABASE_URL`, `API_TOKEN`, `DB_MAX_CONNS` e `LOG_LEVEL`.
4. IF `DATABASE_URL` estiver ausente ou o banco estiver inacessível na inicialização THEN a API SHALL encerrar com código diferente de zero e log explicativo.
5. The API SHALL limitar o pool de conexões ao banco (padrão 4) e o corpo das requisições a 64 KB, retornando `413 payload_too_large` quando excedido.
6. The API SHALL retornar `400 invalid_json` para JSON malformado, com campos desconhecidos, com tipos errados ou com mais de um valor JSON no corpo.
7. The API SHALL tratar qualquer erro inesperado (incluindo panic) retornando `500 internal_error` com mensagem genérica e registrando o detalhe no log.
8. WHEN receber `SIGTERM` ou `SIGINT` THEN a API SHALL parar de aceitar conexões e encerrar de forma graciosa, aguardando requisições em andamento por até 10 segundos.
9. The API SHALL registrar logs estruturados em JSON no stdout com método, rota, status e duração de cada requisição (rota `/health` apenas em nível `debug`).
10. The API SHALL ser distribuída como imagem Docker mínima com binário estático, executando como usuário não-root, com consumo de memória em repouso alvo abaixo de 30 MB.

## Requisitos não funcionais

| Item | Meta |
|---|---|
| Memória em repouso | < 30 MB (alvo 10–20 MB) |
| Tamanho da imagem Docker | < 25 MB |
| Tempo de resposta (p95, banco na rede privada) | < 100 ms para qualquer rota |
| Dependências externas de código | Apenas `github.com/jackc/pgx/v5` |
| Volume esperado | Até ~10 aparelhos simultâneos, ~200 pedidos por dia |
| Disponibilidade | Melhor esforço; reinício automático em falha |

## Fora de escopo

- Login, perfis de usuário, permissões por usuário.
- Pagamentos, formas de pagamento, emissão fiscal, impressão.
- Relatórios e estatísticas.
- Notificações push e WebSocket/SSE.
- Paginação por cursor (a listagem usa apenas `limit`).
