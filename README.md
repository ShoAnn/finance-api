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
- [ ] Error-returning handler adapter wired to `WriteError`
- [ ] Strict JSON decoder: reject malformed JSON, unknown fields, and trailing data
- [ ] JSON `404` / `405` fallbacks to replace the mux's plain-text defaults
- [ ] Request logging middleware: method, path, status, duration
- [ ] Test helpers: fake `sqlc.Querier` and response assertions
- [ ] Shared validators: required, length, enums, ISO 4217 currency, dates, IDs
- [ ] Validation error accumulator rendering `Validation` details
- [ ] Account DTOs and endpoints: list, create, get, archive, balance, delete
- [ ] Category DTOs and endpoints: list, create, update, delete
- [ ] Transaction DTOs and endpoints: list, create, get, update, delete
- [ ] Unit tests for DTO mapping and validation
- [ ] Handler tests for accounts, categories, and transactions
- [ ] Postgres integration harness and constraint error tests
- [ ] CI: build, vet, `go test -race`, and `sqlc diff`
- [ ] Compose smoke test: account → category → transactions → balance
- [ ] Forward `HTTP_*_TIMEOUT` and `DB_*` settings through compose and `.env.example`
- [ ] Makefile targets: `generate`, `migrate`, `test`, `run`
- [ ] README usage docs: endpoints, request examples, error codes
- [ ] Flip `x-implementation-status` to `implemented` and verify the spec
- [ ] Final MVP pass: all of the above verified end to end

## Later (post-MVP)

- [ ] Pagination and filtering for transaction lists
- [ ] Authentication and per-user ownership
- [ ] Transfers between accounts
- [ ] Reporting endpoints (spend by category, monthly totals)
- [ ] OpenAPI server generation or contract tests against the spec
