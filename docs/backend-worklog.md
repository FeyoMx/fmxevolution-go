# Backend Worklog

## Scope

This worklog reflects the current SaaS backend worktree under `cmd/api`, `internal/*`, and the legacy bridge dependencies under `pkg/*`.

## Current branch highlights

### SaaS backend foundation already in place

- `cmd/api/main.go` runs the tenant-aware backend
- `internal/*` contains auth, tenancy, instances, CRM, webhooks, AI, broadcast, middleware, repositories, and server wiring
- `migrations/000001_saas_core.sql` exists as the SQL baseline

### Auth and tenancy

- JWT access and refresh flow
- tenant API key auth
- role checks for `owner`, `admin`, and `agent`
- legacy instance-token compatibility mapped back to tenant identity

### Runtime bridge and instance lifecycle

- tenant-scoped instance CRUD
- connect, disconnect, status, and QR flows through the legacy runtime bridge
- advanced settings bridge
- webhook sync and compatibility response shaping

### Messaging work completed in this branch

- stabilized backend text delivery
- added queued text-job status with `queued`, `running`, `sent`, `delivered`, and `read`
- improved delivery receipt tracking
- added runtime recipient resolution before text send
- implemented tenant-safe SaaS media sending on:
  - `POST /instance/:id/messages/media`
- implemented tenant-safe SaaS audio sending on:
  - `POST /instance/:id/messages/audio`
- implemented live runtime-backed chat list on:
  - `POST /instance/:id/chats/search`
- added legacy compatibility routes for the current frontend chat composer:
  - `POST /message/sendText/:instanceName`
  - `POST /message/sendMedia/:instanceName`
  - `POST /message/sendWhatsAppAudio/:instanceName`
  - `POST /chat/findChats/:instanceName`

### Connector work already completed

- implemented tenant-safe instance routes for:
  - websocket
  - rabbitmq
  - proxy
- kept unsupported suites explicit as `501 partial` instead of pretending parity

## Why these changes were made

- to move the fork toward practical Evolution Go / Manager parity without reviving unsafe global legacy routes
- to satisfy the current sibling frontend’s real dependencies first
- to preserve tenant safety and current SaaS architecture
- to keep unsupported surfaces explicit instead of silently failing or fake-completing them

## Important remaining gaps

- full message-history search is still unsupported
- Chatwoot, SQS, and manager-style integration suites remain explicit `501 partial`
- dashboard metrics still include placeholders
- runtime parity still depends heavily on the legacy bridge

## Files changed in this wave

High-signal files updated for this phase include:

- `internal/instance/chat_media_types.go`
- `internal/instance/compat_handler.go`
- `internal/instance/integration_handler.go`
- `internal/instance/runtime.go`
- `internal/instance/service.go`
- `internal/server/server.go`
- `docs/backend-api.md`
- `docs/backend-parity-report.md`
- `docs/backend-parity-plan.md`
- `docs/backend-product-readiness.md`
- `docs/backend-worklog.md`
- `CHANGELOG.md`

## Consistency notes

### Now more aligned

- backend docs reflect that media and audio sending are no longer `501`
- backend docs now call out chat-list parity versus message-history parity separately
- current frontend chat send routes now have explicit backend compatibility routes instead of only SaaS instance routes

### Still intentionally partial

- `POST /instance/:id/messages/search`
- `POST /chat/findMessages/:instanceName`
- `GET/PUT /instance/:id/sqs`
- `GET/PUT /instance/:id/chatwoot`
- all mounted manager bot/integration suites under:
  - `openai`
  - `typebot`
  - `dify`
  - `n8n`
  - `evoai`
  - `evolutionBot`
  - `flowise`

## Verification notes

- code was reformatted with `gofmt`
- full `go build -o api2.exe ./cmd/api` verification is currently blocked in this environment by the existing `github.com/chai2010/webp` dependency failing to resolve generated symbols during build
- that build issue is environment/dependency-level and not isolated to the new instance parity code
