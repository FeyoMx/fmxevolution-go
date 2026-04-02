# Backend Architecture

## Overview

The backend is currently a hybrid:

- `cmd/api` + `internal/*` provide a newer multi-tenant SaaS API.
- `cmd/evolution-go` + `pkg/*` remain the legacy WhatsApp engine.

The SaaS API stores tenant-scoped metadata in its own PostgreSQL schema and delegates WhatsApp-specific runtime work to the legacy engine through `internal/instance/LegacyRuntime`.

## Folder Structure

```text
cmd/
  api/                 SaaS API entry point
  evolution-go/        Legacy engine entry point
internal/
  ai/                  Tenant AI config, queue, OpenAI calls
  auth/                JWT, API key auth, instance token bridge
  bootstrap/           Default tenant/user bootstrap
  broadcast/           Broadcast jobs, workers, retries, rate pacing
  config/              SaaS API config loader
  crm/                 Contacts, tags, notes
  dashboard/           Instance-based metrics endpoint
  domain/              Shared domain errors and request identity helpers
  handler/             Common HTTP response helpers
  instance/            SaaS instance service + legacy runtime bridge
  middleware/          Auth, tenancy, CORS, logging, rate limiting
  repository/          GORM stores, models, interfaces
  server/              Gin route registration
  service/             Application wiring and background start
  tenant/              Tenant create/get service
  webhook/             Tenant webhook registry + dispatch
migrations/
  000001_saas_core.sql SQL reference schema for the SaaS layer
pkg/
  ...                  Legacy Evolution engine and WhatsApp runtime
```

## Request Flow

1. `cmd/api/main.go` loads `.env` and the SaaS config.
2. `internal/repository.NewStores` opens PostgreSQL and runs GORM `AutoMigrate`.
3. `internal/bootstrap` ensures a default tenant and owner exist.
4. `internal/service.NewApplication` wires auth, tenancy, instances, CRM, webhooks, AI, and broadcast services.
5. `internal/instance.NewLegacyRuntime` attempts to bridge into the legacy engine.
6. `internal/server.New` registers public and protected Gin routes.
7. Protected requests pass through auth middleware, then tenant resolution, then optional role checks and rate limiting.

## Multi-Tenant Model

Primary SaaS tables:

- `tenants`
- `users`
- `instances`
- `contacts`
- `tags`
- `notes`
- `broadcast_jobs`
- `webhook_endpoints`
- `webhook_deliveries`
- `ai_settings`
- `ai_conversation_messages`

Tenant scoping is enforced in two layers:

- the auth identity carries a `tenant_id`
- repositories query by `tenant_id` wherever the SaaS data model is authoritative

The legacy runtime bridge resolves or creates a corresponding legacy instance record by:

- `engine_instance_id`, then
- instance `name`

## Authentication Model

### JWT access and refresh

- Access and refresh tokens are generated in `internal/auth/jwt.go`
- Tokens are stateless HMAC-SHA256 signed strings
- Access token claims carry `sub`, `tenant_id`, `email`, `role`, `type`, `exp`, `iat`
- Refresh tokens reuse the same format with `type=refresh`

Limitations:

- there is no token revocation store
- logout is currently a stateless success response only

### API keys

Tenant API keys are:

- generated once on tenant creation
- stored as bcrypt hashes plus a searchable prefix
- accepted through `X-API-Key` or `apikey`

If tenant API key verification fails, auth can fall back to a legacy instance token resolver that maps a legacy instance token to the owning tenant.

## RBAC

Roles:

- `owner`
- `admin`
- `agent`

Typical access:

- `owner` / `admin`: mutating tenant and instance operations
- `agent`: read access plus operational routes allowed by route definitions

Role checks are enforced per route through `middleware.RequireRoles`.

## Rate Limiting

Implemented in `internal/middleware/rate_limit.go`.

Policies:

- broadcast creation per hour
- webhook dispatch calls per minute
- generic message policy helper exists, but is not currently attached in `server.go`

Backends:

- `memory`: implemented
- `redis`: placeholder only; selecting it currently falls back to in-memory store behavior

Tenant limits are always applied first. When an instance id can be extracted, an additional instance-scoped limit is also applied.

## Webhook Architecture

There are two webhook layers:

1. SaaS-managed webhook endpoints in `internal/webhook`
   - stores endpoint URL, direction flags, optional signing secret
   - dispatches JSON envelopes to all enabled endpoints for a tenant
2. Legacy instance webhook bridge
   - syncs instance-level webhook URL and event subscription into the legacy runtime
   - keeps old frontend payloads working

Supported SaaS directions:

- `inbound`
- `outbound`

Inbound dispatch can also trigger the AI worker queue.

## AI Auto Reply

The AI module is implemented but partial:

- tenant-level provider/model/system prompt config is stored
- per-instance `enabled` and `auto_reply` toggles are stored on the SaaS `instances` table
- inbound webhook payloads can enqueue AI work
- message memory is stored in `ai_conversation_messages`
- reply generation uses the configured OpenAI-compatible endpoint
- generated replies are emitted as outbound webhook events

Current limitation:

- the AI module does not directly send a WhatsApp reply through the legacy engine yet
- it only emits a webhook event describing the generated reply

## Instance Runtime Bridge

`internal/instance/LegacyRuntime` bridges the SaaS instance record to the legacy instance model.

Responsibilities:

- connect/disconnect
- QR retrieval
- status snapshot
- webhook sync
- advanced settings sync
- token / profile / JID / event alias enrichment

This bridge is also where frontend compatibility behavior lives:

- `response.data` + `response.data.data` compatible envelopes
- instance token aliases like `apikey`, `apiKey`, `token`
- advanced settings passthrough (`ignoreGroups`, `ignoreStatus`, etc.)

## Database Strategy

The SaaS API currently relies on `gorm.AutoMigrate` at startup.

`migrations/000001_saas_core.sql` exists as:

- a reference schema
- a manual bootstrap aid

It is not executed automatically by `cmd/api`.

The legacy runtime still relies on its own config/database initialization path in `pkg/config`.

## Bootstrap and Seed Behavior

`internal/bootstrap/seed.go` ensures:

- tenant slug `fmx`
- default owner user `contacto@fmxaiflows.online`

It also optionally resets that default password to `admin123` when:

- `APP_ENV != production`
- `RESET_ADMIN_PASSWORD=true`

## Metrics and Quotas

Implemented:

- simple dashboard instance counts
- rate limiting middleware

Not implemented as a dedicated subsystem:

- tenant quotas
- usage billing
- persistent metrics aggregation

## Known Limitations and TODOs

- Swagger files under `docs/` are stale relative to `cmd/api`
- broadcast processor is a stub
- refresh tokens are stateless and not revocable
- no Redis implementation behind the rate-limit abstraction
- runtime bridge makes some SaaS endpoints unavailable if the legacy engine cannot initialize
- advanced settings are stored in the legacy instance model, not the SaaS `instances` table
