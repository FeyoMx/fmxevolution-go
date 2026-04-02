# Backend Worklog

## Scope audited

This worklog reflects the current branch/worktree state as audited from the backend repository on `main`, with special attention to the new SaaS API surface under `cmd/api`, `internal/`, and `migrations/`.

## What changed

### New SaaS backend surface

- Added `cmd/api/main.go` as a second backend entry point
- Added `internal/*` application modules for:
  - auth
  - tenancy
  - instances
  - CRM
  - webhooks
  - AI
  - broadcasts
  - middleware
  - dashboard metrics
  - repository/store layer
  - application wiring
- Added `migrations/000001_saas_core.sql`

### Auth and tenancy

- Added JWT access and refresh tokens
- Added tenant API key auth
- Added tenant middleware and role checks
- Added legacy instance token fallback auth

### Instance/runtime compatibility

- Added SaaS instance CRUD and bridge methods for:
  - connect
  - disconnect
  - status
  - QR code
  - webhook sync
  - advanced settings sync
- Added response normalization so frontends can consume both:
  - `response.data`
  - `response.data.data`

### Webhooks and AI

- Added tenant webhook endpoint management
- Added inbound/outbound dispatch routes
- Added legacy-compatible webhook payload parsing
- Added AI settings, memory storage, worker queue, OpenAI-compatible reply generation, and outbound webhook emission for generated replies

### Recent fixes in this worktree

- Guarded duplicate concurrent instance startup in the legacy runtime bridge
- Added media-storage fallback when MinIO/S3 is flagged on but runtime storage is nil
- Added webhook payload compatibility for nested legacy webhook objects
- Added advanced settings routes to persist `ignoreGroups` and `ignoreStatus`
- Added instance token aliases in instance/status/QR payloads
- Disabled noisy global webhook usage by allowing empty `WEBHOOK_URL`
- Added tenant-safe instance-scoped integration routes from the frontend gap report
- Fully implemented `websocket`, `rabbitmq`, and `proxy` instance routes through the legacy runtime bridge
- Registered explicit `501 partial` routes for unsupported chat lookup and integration suites instead of reviving legacy `:instanceName` APIs

## Why it was changed

- To introduce a tenant-aware SaaS API without removing the working WhatsApp engine
- To let frontend code move toward tenant-scoped auth and REST resources
- To keep older frontend flows alive long enough through compatibility adapters
- To make webhook and AI flows operational on top of the new tenant model
- To reduce operational instability from duplicated runtime startup and noisy global webhook fan-out

## Files touched

High-signal backend areas touched in this branch/worktree include:

- `cmd/api/main.go`
- `internal/auth/*`
- `internal/bootstrap/seed.go`
- `internal/broadcast/*`
- `internal/config/config.go`
- `internal/crm/*`
- `internal/dashboard/handler.go`
- `internal/instance/*`
- `internal/middleware/*`
- `internal/repository/*`
- `internal/server/server.go`
- `internal/service/app.go`
- `internal/tenant/*`
- `internal/webhook/*`
- `migrations/000001_saas_core.sql`
- `pkg/logger/structured.go`
- `pkg/whatsmeow/service/whatsmeow.go`
- `.env.example`
- `docker/examples/.env.example`
- `README.md`
- `CHANGELOG.md`
- `docs/backend-architecture.md`
- `docs/backend-api.md`
- `docs/backend-env.md`
- `docs/backend-worklog.md`

## Breaking changes / compatibility notes

- The new SaaS API does not expose the old public/manager route set documented in the legacy README and swagger assets.
- New protected routes require JWT or tenant API key auth; some frontend paths still assume old unauthenticated or different manager flows.
- Advanced instance settings now have explicit SaaS routes:
  - `GET /instance/:id/advanced-settings`
  - `PUT /instance/:id/advanced-settings`
- Some older frontend assumptions were handled through compatibility shims:
  - instance webhook payload formats
  - status/QR response envelopes
  - instance token aliases

## Consistency findings

### Updated

- Root README now matches the current backend architecture instead of the legacy public product positioning
- Env examples now align with `internal/config` and the legacy bridge config actually read by the code
- API docs now reflect registered Gin routes from `internal/server/server.go`

### Still partial or stale

- `docs/swagger.json`, `docs/swagger.yaml`, and `docs/docs.go` remain stale relative to `cmd/api`
- broadcast processing is still a stub
- dashboard metrics include placeholder counters
- SQL migration file exists, but startup still uses GORM auto-migration
- most legacy instance integration pages remain partial because the current SaaS backend has no tenant-safe repository/runtime model for Chatwoot, OpenAI bot CRUD, Typebot, Dify, N8N, EvoAI, Evolution Bot, Flowise, SQS, or legacy chat history search

## Follow-up tasks for frontend adaptation

- Move advanced settings toggles to `GET/PUT /instance/:id/advanced-settings`
- Move instance integration pages to the new `/instance/:id/...` SaaS routes
- Treat `501` partial integration responses as unsupported UI states instead of generic failures
- Normalize frontend data access to prefer `response.data.data ?? response.data`
- Fix routes that concatenate `/manager/...//manager/...`
- Ensure protected instance/settings queries always use the authenticated Axios client
- Decide whether dashboard placeholder counters should be hidden or labeled partial in UI

## Suggested commit grouping

1. `docs(backend): document architecture, API and environment`
2. `feat(backend): expose advanced settings routes for tenant instances`
3. `fix(backend): harden runtime bridge and legacy compatibility`

These are proposed groupings only; actual staging should keep generated artifacts and local runtime files out of Git.
