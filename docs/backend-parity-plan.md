# Backend Parity Plan

Prepared on 2026-04-05 from `docs/backend-parity-report.md`.

This plan keeps the current SaaS architecture, tenant scoping, JWT/API key auth, and legacy runtime bridge compatibility. A phase is only considered done when its routes are truly functional; otherwise the route should stay explicit about partial support.

## Phase 1: Core Message / Runtime Parity

Goal:

- finish the most product-critical manager/runtime surfaces without breaking tenancy or reviving unsafe legacy handlers

Exact routes:

- `POST /instance/:id/messages/text`
- `GET /instance/:id/messages/text/:jobID`
- `POST /message/sendText/:instanceName`
- `POST /instance/:id/messages/media`
- `POST /instance/:id/messages/audio`
- `POST /message/sendMedia/:instanceName`
- `POST /message/sendWhatsAppAudio/:instanceName`
- `POST /instance/:id/connect`
- `POST /instance/:id/disconnect`
- `GET /instance/:id/status`
- `GET /instance/:id/qrcode`
- future parity additions:
  - `POST /instance/:id/reconnect`
  - `POST /instance/:id/pair`
  - `DELETE /instance/:id/logout`

Repositories / services to touch:

- `internal/instance/service.go`
- `internal/instance/runtime.go`
- `internal/instance/handler.go`
- `internal/server/server.go`
- `internal/auth/instance_tokens.go`
- `pkg/whatsmeow/service/whatsmeow.go`

Runtime dependencies:

- live WhatsApp client in legacy bridge
- legacy runtime bridge media/audio upload path
- delivery receipt registry

Blockers:

- remaining reconnect/pair/logout parity still needs a safe SaaS route contract
- runtime remains dependent on legacy client state

Test strategy:

- handler validation tests for text/media/audio payloads
- service tests for tenant resolution and auth compatibility where practical
- manual runtime verification against real instance for text/media/audio success and receipt tracking

## Phase 2: Chat / Search / Media Parity

Goal:

- replace the current manager chat UX blockers with a truthful, tenant-safe read model

Exact routes:

- `POST /instance/:id/chats/search`
- `POST /chat/findChats/:instanceName`
- `POST /instance/:id/messages/search`
- `POST /chat/findMessages/:instanceName`
- optional later:
  - chat pin/archive/mute compatibility routes if frontend still needs them

Repositories / services to touch:

- `internal/instance/service.go`
- `internal/instance/runtime.go`
- `internal/repository/*`
- new tenant-safe chat/message repository modules under `internal/`
- possibly webhook ingestion or runtime event persistence

Runtime dependencies:

- live contacts/groups fetch
- receipt and message event ingestion
- possible media-storage references when rendering historical media

Blockers:

- no current tenant-safe `Message[]` history store
- current legacy message repository stores status metadata, not conversation history

Test strategy:

- repository tests for new chat/message read model
- fixture-based handler tests for chats/message search
- end-to-end verification with seeded message history

## Phase 3: Integration Parity

Goal:

- implement only the manager integration suites that are real product priorities and can be made tenant-safe

Exact routes:

- `GET/PUT /instance/:id/chatwoot`
- `GET/PUT /instance/:id/sqs`
- selected high-priority suites from:
  - `openai`
  - `typebot`
  - `dify`
  - `n8n`
  - `evoai`
  - `evolutionBot`
  - `flowise`

Repositories / services to touch:

- new per-integration repositories under `internal/repository`
- `internal/instance/integration_handler.go`
- `internal/instance/service.go`
- integration-specific service packages under `internal/`
- webhook or background worker wiring where sessions/status need persistence

Runtime dependencies:

- per-instance runtime hooks
- session/state persistence
- secure secret storage for external API keys and webhook URLs

Blockers:

- current backend has no tenant-safe model for these suites
- blindly exposing legacy manager handlers would violate SaaS boundaries

Test strategy:

- CRUD/service tests per integration
- auth and tenant-boundary tests
- contract tests for frontend-expected request/response shapes

## Phase 4: Metrics / Analytics / Cleanup

Goal:

- finish the non-core parity work needed for trustworthy ops and product reporting

Exact routes:

- `GET /dashboard/metrics`
- broadcast status/reporting routes already mounted
- documentation and compatibility artifacts:
  - `docs/backend-api.md`
  - `docs/backend-product-readiness.md`
  - `docs/backend-worklog.md`
  - Swagger artifacts if they are brought back into scope

Repositories / services to touch:

- `internal/dashboard/*`
- `internal/broadcast/*`
- `internal/repository/*`
- docs and generated API artifacts

Runtime dependencies:

- stable event persistence
- real aggregates for delivery, contact, and instance usage

Blockers:

- current analytics inputs are incomplete
- several counters are still placeholders

Test strategy:

- repository aggregate tests
- handler tests for metrics payloads
- smoke verification against seeded tenant data

## Immediate Execution Order

1. Keep phase 1 routes stable and verify media/audio/text compatibility.
2. Build the message-history read model for phase 2 before claiming chat parity.
3. Pick one integration family at a time for phase 3, starting from actual frontend/business demand.
4. Finish metrics and documentation cleanup in phase 4 after core parity work stops moving.
