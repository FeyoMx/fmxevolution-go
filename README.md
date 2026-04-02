# Backend Overview

This repository currently contains two backend layers:

- `cmd/api` + `internal/*`: the newer multi-tenant SaaS API written around Gin, GORM, JWT auth, tenant-scoped CRUD, webhooks, AI settings, and a bridge into the legacy WhatsApp runtime.
- `cmd/evolution-go` + `pkg/*`: the legacy Evolution/WhatsApp engine that still owns the WhatsApp session lifecycle, QR generation, webhook production, advanced instance settings, and event ingestion.

The active backend work in this branch is centered on the SaaS API. It does not replace the legacy engine yet; instead it orchestrates and syncs with it.

## What Is Implemented

- Multi-tenant tenant/user/instance data model in PostgreSQL via GORM auto-migrations
- JWT access + refresh token login flow
- Tenant API key authentication
- Legacy instance token fallback authentication for protected SaaS routes
- Tenant-scoped RBAC with `owner`, `admin`, `agent`
- Rate limiting middleware for broadcast creation and webhook dispatch
- Instance CRUD plus QR/connect/status bridge into the legacy runtime
- Legacy compatibility shims for instance settings, webhook payloads, and QR/status response shapes
- Tenant webhook endpoint registry plus inbound/outbound dispatch
- AI tenant settings, per-instance AI toggles, async AI reply generation, and outbound webhook emission for generated replies
- CRM contacts/tags/notes
- Broadcast job queueing with claim/retry bookkeeping
- Bootstrap seed for a default tenant and owner user

## Current Limitations

- The SaaS API still depends on the legacy runtime for QR, connection status, advanced settings, and WhatsApp event behavior.
- Broadcast delivery is not wired to WhatsApp sending yet. The processor is currently a stub that marks jobs complete after delegated processing logic is invoked.
- Dashboard metrics are partial. Instance counts are real; several other counters are placeholder zeros.
- Redis rate limiting is not implemented yet. Selecting `RATE_LIMIT_BACKEND=redis` currently falls back to in-memory behavior.
- `migrations/000001_saas_core.sql` exists for reference/manual SQL, but the application itself currently relies on GORM `AutoMigrate`.
- `docs/swagger.*` and `docs/docs.go` still reflect older/legacy API descriptions and should not be treated as the source of truth for the SaaS layer in this branch.

## Runtime Model

### Entry points

- `cmd/api/main.go`: SaaS API entry point
- `cmd/evolution-go/main.go`: legacy engine entry point

### Boot sequence for `cmd/api`

1. Load `.env` with `godotenv`
2. Load SaaS config from `internal/config`
3. Open PostgreSQL stores through `internal/repository`
4. Run GORM auto-migrations
5. Seed default tenant/user if missing
6. Optionally reset the default admin password in non-production when `RESET_ADMIN_PASSWORD=true`
7. Build the application services
8. Try to initialize the legacy runtime bridge
9. Start background workers for broadcast and AI
10. Start the Gin HTTP server

## Quick Start

### Requirements

- Go 1.25+
- PostgreSQL for the SaaS database
- PostgreSQL and SQLite/legacy dependencies required by the legacy runtime bridge
- A valid `.env` file. See `.env.example` and `docs/backend-env.md`

### Local run

```powershell
go build -o api.exe ./cmd/api
.\api.exe
```

### Health check

```powershell
Invoke-WebRequest -UseBasicParsing http://localhost:8080/healthz
```

## Auth Overview

### Public routes

- `POST /auth/login`
- `POST /auth/refresh`
- `POST /tenant`
- `GET /healthz`

### Protected routes

Protected routes accept either:

- `Authorization: Bearer <access_token>`
- `X-API-Key: <tenant_api_key>`
- `apikey: <tenant_api_key>`

There is also a compatibility fallback that accepts a legacy instance token and resolves it to the owning tenant identity.

## Documentation Map

- [docs/backend-architecture.md](docs/backend-architecture.md)
- [docs/backend-api.md](docs/backend-api.md)
- [docs/backend-env.md](docs/backend-env.md)
- [docs/backend-worklog.md](docs/backend-worklog.md)
- [CHANGELOG.md](CHANGELOG.md)

## Branch Status Notes

This branch contains a large untracked/new SaaS surface under `cmd/api`, `internal/`, and `migrations/` compared with the legacy branch baseline. The documentation in this repo has been updated to describe what is actually implemented in code today, including partial areas and known gaps.
