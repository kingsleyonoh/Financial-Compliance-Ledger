# Financial Compliance Ledger — Codebase Context

> **Immutable, event-sourced audit trail for financial discrepancies.**
> A Go backend service that ingests discrepancy events, enforces escalation policies, tracks resolution workflows, and generates compliance reports — all scoped per-tenant with append-only event sourcing.
>
> Last updated: 2026-04-10
> Template synced: 2026-04-10

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.22+ |
| Framework | Chi router (HTTP) + standard library |
| Database | PostgreSQL 16 (append-only event tables) |
| Event Bus | NATS JetStream (durable event consumption) |
| Report Gen | go-pdf (wkhtmltopdf wrapper) |
| Hosting | Docker on Hetzner VPS |
| Query Gen | sqlc (type-safe SQL) |
| Migrations | golang-migrate |
| Logging | zerolog (structured JSON) |
| Metrics | Prometheus |
| Test Runner | `go test ./...` + testify |

## Project Structure

```
financial-compliance-ledger/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, wires dependencies
├── internal/
│   ├── api/
│   │   ├── router.go            # Chi router setup, middleware registration
│   │   ├── middleware/
│   │   │   ├── tenant.go        # API key → tenant_id resolution
│   │   │   ├── logging.go       # Request/response logging
│   │   │   └── ratelimit.go     # Per-endpoint rate limiting
│   │   └── handlers/
│   │       ├── discrepancies.go # Discrepancy CRUD + workflow actions
│   │       ├── rules.go         # Escalation rule CRUD
│   │       ├── reports.go       # Report generation + download
│   │       ├── stats.go         # Aggregate statistics
│   │       └── health.go        # Health check endpoint
│   ├── domain/
│   │   ├── discrepancy.go       # Discrepancy entity + status machine
│   │   ├── ledger_event.go      # Immutable event entity
│   │   ├── escalation_rule.go   # Escalation rule entity
│   │   └── report.go            # Report entity
│   ├── store/
│   │   ├── postgres.go          # PostgreSQL connection + migrations
│   │   ├── discrepancy_store.go # Discrepancy queries (sqlc)
│   │   ├── event_store.go       # Append-only event queries
│   │   ├── rule_store.go        # Escalation rule queries
│   │   └── report_store.go      # Report queries
│   ├── engine/
│   │   ├── escalation.go        # Escalation evaluation + execution
│   │   ├── ingestion.go         # NATS event consumer
│   │   └── rag_feeder.go        # RAG Platform sync
│   ├── notify/
│   │   └── hub_client.go        # Notification Hub REST client
│   ├── report/
│   │   ├── generator.go         # PDF generation logic
│   │   └── templates/           # HTML templates for reports
│   └── config/
│       └── config.go            # Env var loading, defaults
├── migrations/
│   ├── 001_create_tenants.up.sql
│   └── ...                      # Sequential migration files
├── sqlc/
│   ├── sqlc.yaml                # sqlc configuration
│   └── queries/                 # SQL query files
├── data/
│   └── reports/                 # Generated PDF reports (gitignored)
├── scripts/
│   ├── seed_tenant.sh
│   └── generate_test_events.sh
├── tests/
│   └── e2e/
│       ├── api/                 # E2E API tests
│       └── helpers/             # Test utilities
├── Dockerfile
├── docker-compose.yml
├── docker-compose.prod.yml
├── .env.example
├── go.mod
└── Makefile
```

## Key Modules

| Module | Purpose | Key Files |
|--------|---------|-----------|
| Event Ingestion | Consume discrepancy events from NATS JetStream | `internal/engine/ingestion.go` |
| Escalation Engine | Evaluate rules, fire actions (notify/escalate/auto_close) | `internal/engine/escalation.go` |
| Resolution Workflow | State machine for discrepancy lifecycle | `internal/domain/discrepancy.go`, `internal/api/handlers/discrepancies.go` |
| Report Generator | HTML→PDF compliance reports via wkhtmltopdf | `internal/report/generator.go` |
| RAG Feeder | Push resolved discrepancies to RAG Platform | `internal/engine/rag_feeder.go` |
| Notification Hub Client | REST client for escalation alerts | `internal/notify/hub_client.go` |
| Tenant Middleware | API key → tenant_id resolution + caching | `internal/api/middleware/tenant.go` |

## Database Schema

| Table | Purpose | Key Fields |
|-------|---------|-----------|
| tenants | Multi-tenant isolation | id (UUID), name, api_key (hashed), is_active, settings (JSONB) |
| discrepancies | Tracked financial discrepancies | id, tenant_id, external_id, severity, status, amount_expected/actual |
| ledger_events | Immutable append-only audit trail | id, tenant_id, discrepancy_id, event_type, actor, payload, sequence_num |
| escalation_rules | Configurable escalation policies | id, tenant_id, severity_match, trigger_after_hrs, action, action_config |
| reports | Generated compliance report metadata | id, tenant_id, report_type, status, file_path |
| notification_log | Outbound notification tracking | id, tenant_id, discrepancy_id, channel, status, attempts |

## External Integrations

| Service | Purpose | Auth Method |
|---------|---------|------------|
| Transaction Reconciliation Engine | Source of discrepancy events (via NATS) | NATS token |
| Event-Driven Notification Hub | Escalation alert delivery | API Key (`X-API-Key` header) |
| Multi-Agent RAG Platform | Queryable compliance history feed | API Key (`X-API-Key` header) |
| Prometheus | Metrics collection | None (scrape endpoint) |
| BetterStack | Uptime monitoring | External (configured separately) |

## Ecosystem Connections

| Direction | Connected System | Method | Endpoint / Topic | Env Var |
|-----------|-----------------|--------|-------------------|---------|
| ← inbound | Transaction Reconciliation Engine | NATS JetStream | `recon.discrepancy.detected` | `NATS_URL`, `NATS_TOKEN` |
| → outbound | Event-Driven Notification Hub | REST | `POST /api/events` | `NOTIFICATION_HUB_URL`, `NOTIFICATION_HUB_API_KEY` |
| → outbound | Multi-Agent RAG Platform | REST | `POST /api/documents` | `RAG_PLATFORM_URL`, `RAG_PLATFORM_API_KEY` |

## Environment Variables

| Variable | Purpose | Source |
|----------|---------|--------|
| DATABASE_URL | PostgreSQL connection string | `.env` |
| NATS_URL | NATS server address | `.env` |
| NATS_TOKEN | NATS authentication | `.env` |
| PORT | HTTP server port (default: 8080) | `.env` |
| LOG_LEVEL | Logging level (default: info) | `.env` |
| NOTIFICATION_HUB_URL | Notification Hub base URL | `.env` |
| NOTIFICATION_HUB_API_KEY | Hub authentication | `.env` |
| NOTIFICATION_HUB_ENABLED | Enable Hub integration (default: false) | `.env` |
| RAG_PLATFORM_URL | RAG Platform base URL | `.env` |
| RAG_PLATFORM_API_KEY | RAG authentication | `.env` |
| RAG_FEED_ENABLED | Enable RAG feed (default: false) | `.env` |
| ESCALATION_INTERVAL_MINUTES | Escalation check frequency (default: 15) | `.env` |
| MAX_NOTIFICATION_RETRIES | Max retry attempts (default: 3) | `.env` |
| SELF_REGISTRATION_ENABLED | Allow tenant self-registration (default: true) | `.env` |
| REPORT_STORAGE_PATH | PDF storage directory (default: data/reports) | `.env` |
| REPORT_MAX_EVENTS | Max events per report (default: 10000) | `.env` |

## Commands

| Action | Command |
|--------|---------|
| Dev server | `go run cmd/server/main.go` |
| Run tests | `go test ./...` |
| Run tests (verbose) | `go test -v ./...` |
| Test coverage | `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out` |
| Lint/check | `go vet ./... && golangci-lint run` |
| Build | `go build -o bin/server cmd/server/main.go` |
| Migrate DB | Auto-runs on server startup via `golang-migrate` |
| Generate sqlc | `sqlc generate` |
| E2E tests | `make test-e2e` |

## Tenant Model

- **Isolation strategy:** API key in `X-API-Key` header → lookup `tenants` table → inject `tenant_id` into request context
- **Tenant table:** `tenants` with hashed API keys, JSONB settings
- **Middleware:** `internal/api/middleware/tenant.go` — resolves API key to tenant, injects into context
- **Cache:** In-memory tenant cache with 5-minute TTL, invalidated on tenant update
- **Self-registration:** `POST /api/tenants/register` (controlled by `SELF_REGISTRATION_ENABLED`)

## Key Patterns & Conventions

- File naming: `snake_case.go` for all Go files
- Package naming: lowercase, no underscores (e.g., `store`, `domain`, `api`)
- Import hierarchy: standard lib → third-party → local packages
- Error handling: structured JSON error responses `{ error: { code, message, details } }`
- Dependency injection: constructor functions wired in `cmd/server/main.go`
- Logging: zerolog structured JSON, includes tenant_id, request_id
- Immutability: `ledger_events` table is append-only — NO updates, NO deletes
- Tenant scoping: every query includes `tenant_id` filter, enforced by middleware

## Gotchas & Lessons Learned

> Discovered during implementation. Added automatically by `/implement-next` Step 9.3.

| Date | Area | Gotcha | Discovered In |
|------|------|--------|---------------|
| 2026-04-10 | PostgreSQL | Docker PG port 5432 may conflict with system PostgreSQL. Map to alternate port (e.g., 5440:5432) in docker-compose.override.yml. Update DATABASE_URL accordingly. | Template KB (Swarm Intelligence Gateway) |

## Shared Foundation (MUST READ before any implementation)

| Category | File(s) | What it establishes |
|----------|---------|-------------------|
| Config loading | `internal/config/config.go` | All env var defaults, validation, struct |
| DB connection | `internal/store/postgres.go` | PostgreSQL pool, auto-migration, graceful shutdown |
| Domain entities | `internal/domain/` | Discrepancy status machine, event types, entity structs |
| Error handling | `internal/api/handlers/` (shared) | Standard error response format `{ error: { code, message, details } }` |
| Tenant middleware | `internal/api/middleware/tenant.go` | API key → tenant_id resolution, caching, context injection |
| Logging | `internal/api/middleware/logging.go` | Zerolog request/response structured logging |
| NATS client | `internal/engine/ingestion.go` | JetStream connection, durable consumer lifecycle |
| Hub client | `internal/notify/hub_client.go` | HTTP client with retry logic for Notification Hub |

## Deep References

| Topic | Where to look |
|-------|--------------|
| Discrepancy lifecycle | `internal/domain/discrepancy.go` |
| Event sourcing | `internal/store/event_store.go` |
| Escalation rules | `internal/engine/escalation.go` |
| NATS integration | `internal/engine/ingestion.go` |
| PDF reports | `internal/report/generator.go` |
| Tenant isolation | `internal/api/middleware/tenant.go` |
| API routes | `internal/api/router.go` |
| Database schema | `migrations/` |
| Test patterns | `tests/`, `internal/*/..._test.go` |
