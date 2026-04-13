# Financial Compliance Ledger — Immutable audit trail for financial discrepancies

Built by [Kingsley Onoh](https://kingsleyonoh.com) · Systems Architect

## The Problem

When a transaction reconciliation engine flags a discrepancy — a mismatched amount, a missing settlement, a timing gap — that finding needs to survive audits. Regulators in PSD2 and MaRisk environments expect a tamper-proof record of what was found, when it was found, who looked at it, and how it was resolved. Most teams track this in spreadsheets or ticketing systems where records can be edited, deleted, or lose context. One disputed transaction can mean weeks of reconstructing history from logs.

This service provides an append-only, event-sourced ledger where every discrepancy state change — received, acknowledged, investigated, resolved — is an immutable event. Nothing is updated. Nothing is deleted. The audit trail is the source of truth.

## Architecture

```mermaid
%%{init: {'theme':'base','themeVariables':{'primaryColor':'#3B82F6','primaryTextColor':'#F0F0F5','primaryBorderColor':'#3B82F6','lineColor':'#3B82F6','secondaryColor':'#141418','tertiaryColor':'#0D0D0F','background':'#0D0D0F','mainBkg':'#141418','nodeBorder':'#3B82F6','clusterBkg':'#0D0D0F','clusterBorder':'#33333F','titleColor':'#F0F0F5','edgeLabelBackground':'#141418'}}}%%
graph TB
    RE[Transaction Reconciliation Engine] -->|NATS JetStream| ING[Event Ingestion]

    subgraph Compliance Ledger
        ING --> DS[(PostgreSQL 16)]
        API[Chi REST API] --> DS
        ESC[Escalation Engine] --> DS
        RPT[Report Generator] --> DS
        ESC -->|every 15 min| DS
    end

    API -->|tenant auth| CLIENT[API Consumers]
    ESC -->|alerts| NH[Notification Hub]
    RPT -->|PDF/HTML| CLIENT
    DS -->|resolved events| RAG[RAG Platform]
    PROM[Prometheus] -->|scrape /metrics| API
```

## Key Decisions

- **I chose append-only event sourcing over mutable status fields** because an audit trail that allows UPDATEs is not an audit trail. Every state change is a new row in `ledger_events` with a monotonic sequence number. The discrepancy status is derived state, not source of truth.

- **I chose NATS JetStream over Kafka** because this system processes hundreds of events per second, not millions. JetStream gives durable consumption, replay, and dead-letter handling in a single 20MB binary — no ZooKeeper, no broker clusters, no operational overhead.

- **I chose pgx over GORM** because the query patterns are simple (tenant-scoped CRUD with cursor pagination) and the performance cost of an ORM's reflection layer isn't worth the convenience. Parameterized SQL with `pgx` gives full control and eliminates the ORM-generated-query debugging problem.

- **I chose per-tenant API key auth over JWT** because this is a service-to-service API, not a user-facing app. API keys hashed with SHA-256 and cached with a 5-minute TTL are simpler to manage, rotate, and revoke than JWT token infrastructure.

## Setup

### Prerequisites

- Go 1.22+
- Docker and Docker Compose (for PostgreSQL 16 and NATS)

### Installation

```bash
git clone https://github.com/kingsleyonoh/Financial-Compliance-Ledger.git
cd Financial-Compliance-Ledger
cp .env.example .env
docker compose up -d
go run cmd/server/main.go
```

### Environment

```bash
cp .env.example .env
```

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://fcl:localdev@localhost:5441/compliance_ledger` |
| `NATS_URL` | NATS server address | `nats://localhost:4222` |
| `PORT` | HTTP server port | `8080` |
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | `info` |
| `ESCALATION_INTERVAL_MINUTES` | How often the escalation engine runs | `15` |
| `SELF_REGISTRATION_ENABLED` | Allow tenant self-registration | `true` |
| `NOTIFICATION_HUB_ENABLED` | Enable Notification Hub integration | `false` |
| `RAG_FEED_ENABLED` | Enable RAG Platform feed | `false` |
| `REPORT_STORAGE_PATH` | Directory for generated reports | `data/reports` |

### Run

```bash
# Start dependencies
docker compose up -d

# Start the server
go run cmd/server/main.go

# Or build and run
make build && ./bin/server
```

## How It Works

```
Recon Engine ──NATS──► Ingestion ──► PostgreSQL
                         │              │
                         ▼              │
                    Deduplicate         │
                    (tenant_id +        │
                     external_id)       ▼
                                   ┌─────────┐
                        API ◄──────│ Ledger   │
                         │         │ Events   │
                         ▼         └─────────┘
                    ┌─────────┐         ▲
                    │ Workflow │─────────┘
                    │ Actions  │  (append event
                    └─────────┘   per action)
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
          Acknowledge  Investigate  Resolve
              │          │          │
              └──────────┼──────────┘
                         ▼
                  Escalation Engine
                  (rules × schedule)
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
           Notify    Escalate    Auto-close
```

1. The Transaction Reconciliation Engine publishes `discrepancy.detected` events to NATS JetStream.
2. The ingestion consumer validates, deduplicates by `(tenant_id, external_id)`, and persists to PostgreSQL.
3. Each discrepancy starts as `open` and moves through `acknowledged → investigating → resolved` via API calls.
4. Every action appends an immutable event to `ledger_events` — the full history is always queryable.
5. The escalation engine evaluates configurable rules every 15 minutes, firing alerts or auto-closing stale discrepancies.
6. Compliance reports aggregate discrepancy data into downloadable HTML/PDF documents.

## Usage

### Register a tenant and get an API key

```bash
curl -X POST http://localhost:8080/api/tenants/register \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Financial"}'
```

```json
{
  "id": "9e8d6e9e-ddb0-457c-900e-d48c67369659",
  "name": "Acme Financial",
  "api_key": "fcl_live_50427133a2c88798..."
}
```

Save the `api_key` — it's shown only once. Use it in the `X-API-Key` header for all authenticated requests.

### View discrepancies

```bash
# List with filters
curl http://localhost:8080/api/discrepancies?status=open&severity=high \
  -H "X-API-Key: $API_KEY"

# Get one with full event timeline
curl http://localhost:8080/api/discrepancies/$ID \
  -H "X-API-Key: $API_KEY"
```

The detail endpoint returns the discrepancy and its complete, ordered event history:

```json
{
  "discrepancy": { "id": "...", "status": "investigating", "severity": "high", ... },
  "events": [
    { "event_type": "discrepancy.received", "actor": "nats-ingestion", "sequence_num": 1, ... },
    { "event_type": "discrepancy.acknowledged", "actor": "analyst@acme.com", "sequence_num": 2, ... },
    { "event_type": "discrepancy.investigation_started", "actor": "analyst@acme.com", "sequence_num": 3, ... }
  ]
}
```

### Work a discrepancy

```bash
# Acknowledge
curl -X POST .../discrepancies/$ID/acknowledge \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"actor": "analyst@acme.com"}'

# Start investigation
curl -X POST .../discrepancies/$ID/investigate \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"actor": "analyst@acme.com", "notes": "Checking gateway logs"}'

# Add a note
curl -X POST .../discrepancies/$ID/notes \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"actor": "analyst@acme.com", "content": "Found matching transaction"}'

# Resolve
curl -X POST .../discrepancies/$ID/resolve \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"actor": "analyst@acme.com", "resolution_type": "match_found"}'
```

Resolution types: `match_found`, `false_positive`, `manual_adjustment`, `write_off`.

### Configure escalation rules

```bash
# Create a rule: notify on high-severity discrepancies open > 24 hours
curl -X POST http://localhost:8080/api/rules \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{
    "name": "Escalate high after 24h",
    "severity_match": "high",
    "trigger_after_hrs": 24,
    "trigger_status": "open",
    "action": "notify",
    "action_config": {"channel": "email", "recipient": "ops@acme.com"},
    "priority": 1
  }'
```

Actions: `notify` (send alert via Notification Hub), `escalate` (increase severity), `auto_close` (resolve automatically).

### Generate a compliance report

```bash
# Request generation (returns 202, runs in background)
curl -X POST http://localhost:8080/api/reports \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"report_type": "daily_summary", "date_from": "2026-04-01", "date_to": "2026-04-30"}'

# Download when ready
curl http://localhost:8080/api/reports/$REPORT_ID/download \
  -H "X-API-Key: $API_KEY" -o report.html
```

Report types: `daily_summary`, `monthly_audit`, `discrepancy_detail`.

### API reference

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | No | PostgreSQL + NATS connection status |
| GET | `/metrics` | No | Prometheus metrics |
| POST | `/api/tenants/register` | No | Self-register, returns API key |
| GET | `/api/tenants/me` | Yes | Current tenant profile |
| GET | `/api/discrepancies` | Yes | Filtered list with cursor pagination |
| GET | `/api/discrepancies/:id` | Yes | Detail with event timeline |
| POST | `/api/discrepancies/:id/acknowledge` | Yes | Acknowledge |
| POST | `/api/discrepancies/:id/investigate` | Yes | Start investigation |
| POST | `/api/discrepancies/:id/resolve` | Yes | Resolve with resolution type |
| POST | `/api/discrepancies/:id/notes` | Yes | Add note |
| GET | `/api/rules` | Yes | List escalation rules |
| POST | `/api/rules` | Yes | Create escalation rule |
| PUT | `/api/rules/:id` | Yes | Update escalation rule |
| DELETE | `/api/rules/:id` | Yes | Delete escalation rule |
| GET | `/api/reports` | Yes | List reports |
| POST | `/api/reports` | Yes | Request report generation |
| GET | `/api/reports/:id/download` | Yes | Download report file |
| GET | `/api/stats` | Yes | Aggregate counts by status/severity |

## Tests

```bash
# Run all 346 tests (hits real PostgreSQL + NATS via Docker)
make test

# With coverage
make test-cover

# E2E integration test only
make test-e2e
```

Tests hit real PostgreSQL and real NATS — no mocked databases. The E2E test covers the full pipeline: NATS event → ingestion → workflow lifecycle → escalation → report generation.

## AI Integration

This project includes machine-readable context for AI tools:

| File | What it does |
|------|-------------|
| [`llms.txt`](llms.txt) | Project summary for LLMs ([llmstxt.org](https://llmstxt.org)) |
| [`AGENTS.md`](AGENTS.md) | Full codebase instructions for AI coding agents |
| [`openapi.yaml`](openapi.yaml) | OpenAPI 3.1 API specification |
| [`mcp.json`](mcp.json) | MCP server definition for AI IDEs |

### Cursor / Other AI IDEs
Point your AI agent at `AGENTS.md` for full codebase context.

## Deployment

This project runs on a Hetzner VPS behind Traefik with automatic TLS via Let's Encrypt.

### Production Stack

| Component | Role |
|-----------|------|
| `compliance-ledger` | Go binary (Chi HTTP + NATS consumer + background goroutines) |
| `postgres` | PostgreSQL 16 — append-only ledger storage |
| `nats` | NATS with JetStream — durable event consumption |
| Traefik | Reverse proxy with automatic HTTPS |

### Self-Host

```bash
# Pull the image
docker pull ghcr.io/kingsleyonoh/financial-compliance-ledger:latest

# Or use the compose file
docker compose -f docker-compose.prod.yml up -d
```

Set the environment variables listed in **Setup > Environment** before starting. At minimum: `DATABASE_URL`, `POSTGRES_PASSWORD`, `NATS_URL`.

<!-- THEATRE_LINK -->
