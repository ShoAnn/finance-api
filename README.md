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

## Ticket board

The board **is** the roadmap: epics and tickets are grouped and ordered by
dependency, so the default is top to bottom. Check a box when the ticket is
merged. Ticket IDs are stable — new work appends the next free `FIN-nn`
instead of renumbering.

**Priority:** `P0` blocks MVP · `P1` required before release · `P2` polish
**Type:** `feat` · `test` · `chore` · `docs` · `refactor`
**deps:** tickets that must land first (`—` = none)

### Closed — foundation

- [x] **FIN-01** Schema migration with accounts/categories/transactions and indexes — `feat` · ffa3c21
- [x] **FIN-02** sqlc queries for all resources + balance, generated `Querier` — `feat` · ffa3c21, ad8b34f
- [x] **FIN-03** Config loading, validation, and pool tuning — `feat` · ffa3c21, ad8b34f
- [x] **FIN-04** API error envelope and Postgres error translation — `feat` · ad8b34f
- [x] **FIN-05** Health/readiness endpoints with graceful shutdown — `feat` · ffa3c21
- [x] **FIN-06** Dockerfile and compose stack (db, migrate, api) — `chore` · ffa3c21
- [x] **FIN-07** OpenAPI 3.1 contract for the full v0 scope — `docs` · ad8b34f

### Epic 1 — HTTP core

- [ ] **FIN-10** Router skeleton: move health handlers into `internal/api`, add `api.NewRouter(q sqlc.Querier)` — `refactor` · `P0` · deps: —
- [ ] **FIN-11** Error-returning handler adapter: `func(w, r) error` wired to `apierr.WriteError` — `feat` · `P0` · deps: FIN-10
- [ ] **FIN-12** Strict JSON decoder: reject malformed JSON, unknown fields, and trailing data as `invalid_request` — `feat` · `P0` · deps: —
- [ ] **FIN-13** JSON `404` / `405` fallbacks to replace the mux's plain-text defaults — `feat` · `P1` · deps: FIN-10
- [ ] **FIN-14** Request logging middleware: method, path, status, duration at `info` — `feat` · `P2` · deps: FIN-10
- [ ] **FIN-15** Wire router into `main.go` with injected `Querier`; keep startup/shutdown behavior unchanged — `refactor` · `P0` · deps: FIN-10
- [ ] **FIN-16** Test harness: fake `sqlc.Querier` and response/assert helpers — `test` · `P0` · deps: FIN-11

### Epic 2 — Serialization and validation

- [ ] **FIN-20** Shared validators: required, `minLength`, enum membership, ISO 4217 currency, dates, path IDs — `feat` · `P0` · deps: FIN-12
- [ ] **FIN-21** Field-error accumulator that renders `apierr.Validation` details — `feat` · `P0` · deps: FIN-20
- [ ] **FIN-22** Account DTOs and model mapping (RFC 3339 timestamps, nullable `archived_at`) — `feat` · `P0` · deps: FIN-20
- [ ] **FIN-23** Category DTOs and model mapping — `feat` · `P0` · deps: FIN-20
- [ ] **FIN-24** Transaction DTOs and model mapping (`occurred_on` as `YYYY-MM-DD`, nullable fields as `null`) — `feat` · `P0` · deps: FIN-20
- [ ] **FIN-25** Unit tests for mapping and validation edge cases — `test` · `P0` · deps: FIN-22, FIN-23, FIN-24

### Epic 3 — Accounts

- [ ] **FIN-30** `GET /accounts` — active only, ordered by name — `feat` · `P0` · deps: FIN-16, FIN-22
- [ ] **FIN-31** `POST /accounts` — `201` + `Location`, currency defaults to `USD` — `feat` · `P0` · deps: FIN-16, FIN-22
- [ ] **FIN-32** `GET /accounts/{accountId}` — includes archived accounts, `404` otherwise — `feat` · `P0` · deps: FIN-16, FIN-22
- [ ] **FIN-33** `POST /accounts/{accountId}/archive` — set `archived_at`, return account — `feat` · `P0` · deps: FIN-32
- [ ] **FIN-34** `GET /accounts/{accountId}/balance` — sum of transactions, `0` when empty — `feat` · `P0` · deps: FIN-32
- [ ] **FIN-35** `DELETE /accounts/{accountId}` — map FK violation to `409 conflict` — `feat` · `P0` · deps: FIN-32
- [ ] **FIN-36** Account handler tests: status codes, bodies, `Location`, `404`/`409` paths — `test` · `P0` · deps: FIN-30…FIN-35

### Epic 4 — Categories

- [ ] **FIN-40** `GET /categories` — ordered by name — `feat` · `P0` · deps: FIN-16, FIN-23
- [ ] **FIN-41** `POST /categories` — `201` + `Location`, duplicate name → `409` — `feat` · `P0` · deps: FIN-16, FIN-23
- [ ] **FIN-42** `PUT /categories/{categoryId}` — full replace, `404` / `409` handling — `feat` · `P0` · deps: FIN-41
- [ ] **FIN-43** `DELETE /categories/{categoryId}` — transactions keep `category_id = null` — `feat` · `P0` · deps: FIN-42
- [ ] **FIN-44** Category handler tests, including the `categories_name_key` → `409` mapping — `test` · `P0` · deps: FIN-40…FIN-43

### Epic 5 — Transactions

- [ ] **FIN-50** `GET /transactions?account_id=&from=&to=` — required params, inclusive range, `from <= to`, newest first — `feat` · `P0` · deps: FIN-16, FIN-24
- [ ] **FIN-51** `POST /transactions` — `201` + `Location` — `feat` · `P0` · deps: FIN-16, FIN-24
- [ ] **FIN-52** `GET /transactions/{transactionId}` — `404` on unknown ID — `feat` · `P0` · deps: FIN-16, FIN-24
- [ ] **FIN-53** `PUT /transactions/{transactionId}` — full replace, refresh `updated_at` — `feat` · `P0` · deps: FIN-52
- [ ] **FIN-54** `DELETE /transactions/{transactionId}` — `204` / `404` — `feat` · `P0` · deps: FIN-52
- [ ] **FIN-55** Transaction handler tests: query validation, bad dates, non-numeric IDs — `test` · `P0` · deps: FIN-50…FIN-54

### Epic 6 — Integration and CI

- [ ] **FIN-60** Postgres-backed test harness (compose service or testcontainers) with migrations applied — `test` · `P0` · deps: FIN-15
- [ ] **FIN-61** Integration tests for unique, FK, and check-constraint error mapping — `test` · `P0` · deps: FIN-60, FIN-36, FIN-44
- [ ] **FIN-62** CI workflow: build, vet, `go test -race`, and `sqlc diff` drift check — `chore` · `P1` · deps: FIN-25
- [ ] **FIN-63** Compose smoke test: create account → category → transactions → balance — `test` · `P0` · deps: FIN-61

### Epic 7 — Docs and release

- [ ] **FIN-70** Pass `HTTP_*_TIMEOUT` and `DB_*` settings through `compose.yaml` and `.env.example` — `chore` · `P1` · deps: —
- [ ] **FIN-71** Makefile targets: `generate`, `migrate`, `test`, `run` — `chore` · `P2` · deps: —
- [ ] **FIN-72** README usage docs: endpoints, request examples, error codes — `docs` · `P1` · deps: FIN-36, FIN-44, FIN-55
- [ ] **FIN-73** Flip `x-implementation-status` to `implemented` and confirm spec matches handlers — `docs` · `P0` · deps: FIN-36, FIN-44, FIN-55
- [ ] **FIN-74** MVP sign-off: all exit criteria below verified — `chore` · `P0` · deps: FIN-63, FIN-72, FIN-73

### Parallelism

- Epic 1 is a hard gate for everything else.
- After Epic 2, Epics 3–5 touch separate handler/DTO files and can run in
  parallel by different people or agents.
- Epic 6 starts as soon as the first resource epic is wired; it validates the
  rest as they land.

## MVP exit criteria

- [ ] All 15 resource operations in `openapi.yaml` implemented, none `planned` (FIN-30…FIN-55)
- [ ] Every documented `400` / `404` / `409` response reachable and tested (FIN-36, FIN-44, FIN-55, FIN-61)
- [ ] End-to-end flow works through compose (FIN-63)
- [ ] README documents how to run and exercise the API (FIN-72)

## Parked (post-MVP)

- [ ] Pagination and filtering for transaction lists
- [ ] Authentication and per-user ownership
- [ ] Transfers between accounts
- [ ] Reporting endpoints (spend by category, monthly totals)
- [ ] OpenAPI server generation or contract tests against the spec
