# Finance API

Personal finance tracking API in Go, backed by Postgres. Tracks accounts,
categories, and transactions. The HTTP contract lives in
[`openapi.yaml`](./openapi.yaml).

## Run locally

```sh
cp .env.example .env     # optional; defaults work as-is
docker compose up --build
curl localhost:8080/healthz
curl localhost:8080/readyz
```

The stack starts Postgres, applies `migrations/`, then runs the API on
[`localhost:8080`](http://localhost:8080).

## API keys

Every endpoint except `/healthz` and `/readyz` requires an API key. Create one
key per client:

```sh
make key NAME=laptop                         # local Go toolchain
make key-docker NAME=laptop                  # same, against the compose database
```

The key is printed once and only its SHA-256 hash is stored in `api_keys`.
Send it on every request:

```sh
curl -H "Authorization: Bearer $KEY" localhost:8080/accounts
```

## API

All request and response bodies are JSON. Errors use a consistent envelope:

```json
{"error": {"code": "not_found", "message": "account not found"}}
```

| Method   | Path                              | Description                              |
| -------- | --------------------------------- | ---------------------------------------- |
| `GET`    | `/healthz`                        | Liveness probe                           |
| `GET`    | `/readyz`                         | Readiness probe (pings the database)     |
| `GET`    | `/accounts`                       | List active accounts                     |
| `POST`   | `/accounts`                       | Create an account                        |
| `GET`    | `/accounts/{accountId}`           | Get an account, archived or not          |
| `DELETE` | `/accounts/{accountId}`           | Delete an account                        |
| `POST`   | `/accounts/{accountId}/archive`   | Archive an account                       |
| `GET`    | `/accounts/{accountId}/balance`   | Sum of the account's transactions        |
| `GET`    | `/categories`                     | List categories                          |
| `POST`   | `/categories`                     | Create a category                        |
| `PUT`    | `/categories/{categoryId}`        | Replace a category                       |
| `DELETE` | `/categories/{categoryId}`        | Delete a category                        |
| `GET`    | `/transactions`                   | List an account's transactions by date   |
| `POST`   | `/transactions`                   | Create a transaction                     |
| `GET`    | `/transactions/{transactionId}`   | Get a transaction                        |
| `PUT`    | `/transactions/{transactionId}`   | Replace a transaction                    |
| `DELETE` | `/transactions/{transactionId}`   | Delete a transaction                     |

Amounts are integer cents. Positive transaction amounts are money in, negative
amounts are money out.

### Examples

Set `KEY` to a key created above; every request needs the `Authorization`
header.

```sh
KEY="<key printed by make key>"
```

Create an account (currency defaults to `USD`):

```sh
curl -i -X POST localhost:8080/accounts \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Everyday Checking","type":"checking"}'
```

Create a category:

```sh
curl -i -X POST localhost:8080/categories \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Groceries","kind":"expense"}'
```

Create a transaction (`occurred_on` is `YYYY-MM-DD`):

```sh
curl -i -X POST localhost:8080/transactions \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"account_id":1,"category_id":1,"amount_cents":-4599,"description":"Weekly grocery run","occurred_on":"2026-01-20"}'
```

Read the balance and list transactions:

```sh
curl -H "Authorization: Bearer $KEY" localhost:8080/accounts/1/balance
curl -H "Authorization: Bearer $KEY" 'localhost:8080/transactions?account_id=1&from=2026-01-01&to=2026-01-31'
```

Archive or delete:

```sh
curl -X POST -H "Authorization: Bearer $KEY" localhost:8080/accounts/1/archive
curl -X DELETE -H "Authorization: Bearer $KEY" localhost:8080/transactions/1
```

## Development

```sh
make generate          # regenerate db/sqlc from db/queries
make key NAME=laptop   # create a client API key (printed once)
make test              # unit tests
make test-integration  # requires Postgres with migrations applied
make smoke             # end-to-end test against a running stack
```

`test-integration` uses `TEST_DATABASE_URL`, defaulting to the compose
database. The smoke test uses `BASE_URL`, defaulting to `http://localhost:8080`,
and requires `API_KEY` to be set.

---

## Todo

Roughly in dependency order — work top to bottom.

- [x] Schema migration for accounts, categories, and transactions
- [x] sqlc queries and generated `Querier`
- [x] Config loading, validation, and pool tuning
- [x] API error envelope and Postgres error translation
- [x] Health/readiness endpoints with graceful shutdown
- [x] Dockerfile and compose stack
- [x] OpenAPI 3.1 contract
- [x] Router skeleton and health handlers in `handlers.go`
- [x] Store layer wrapping the generated queries
- [x] Error-returning handler adapter wired to `WriteError`
- [x] Strict JSON decoder: reject malformed JSON, unknown fields, and trailing data
- [x] JSON `404` / `405` fallbacks to replace the mux's plain-text defaults
- [x] Request logging middleware: method, path, status, duration
- [x] Test helpers: fake `sqlc.Querier` and response assertions
- [x] Shared validators: required, length, enums, ISO 4217 currency, dates, IDs
- [x] Validation error accumulator rendering `Validation` details
- [x] Account DTOs and endpoints: list, create, get, archive, balance, delete
- [x] Category DTOs and endpoints: list, create, update, delete
- [x] Transaction DTOs and endpoints: list, create, get, update, delete
- [x] Unit tests for DTO mapping and validation
- [x] Handler tests for accounts, categories, and transactions
- [x] Postgres integration harness and constraint error tests
- [x] CI: build, vet, `go test -race`, and `sqlc diff`
- [x] Compose smoke test: account → category → transactions → balance
- [x] API key auth: hashed key storage, one-off key CLI, per-request check
- [x] Forward `HTTP_*_TIMEOUT` and `DB_*` settings through compose and `.env.example`
- [x] Makefile targets: `generate`, `migrate`, `test`, `run`
- [x] README usage docs: endpoints, request examples, error codes
- [x] Flip `x-implementation-status` to `implemented` and verify the spec
- [x] Final MVP pass: all of the above verified end to end

## Later (post-MVP)

- [ ] Pagination and filtering for transaction lists
- [ ] Per-user ownership and scoped API keys
- [ ] Transfers between accounts
- [ ] Reporting endpoints (spend by category, monthly totals)
- [ ] OpenAPI server generation or contract tests against the spec
