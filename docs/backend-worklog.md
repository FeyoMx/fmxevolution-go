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
- connect, disconnect, reconnect, pair, logout, status, and QR flows through the legacy runtime bridge
- durable runtime session state plus lifecycle event history for instance UX
- tenant-safe runtime observability routes on:
  - `GET /instance/:id/runtime`
  - `GET /instance/:id/runtime/history`
  - plus `/instance/id/:instanceID/*` aliases
- advanced settings bridge
- webhook sync and compatibility response shaping
- runtime admin actions return compatibility envelopes with refreshed status fields so the frontend can use the action response as an immediate operational refresh
- SaaS runtime admin parity is mounted only on tenant-scoped `/instance/:id` and `/instance/id/:instanceID` routes; unsafe legacy global handlers remain unexposed
- durable lifecycle records now capture:
  - `connected`
  - `disconnected`
  - `pairing_started`
  - `paired`
  - `reconnect_requested`
  - `logout`
  - `status_observed`
- replay/backfill checkpoints now also capture:
  - `history_sync_requested`
  - `history_sync`

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
- implemented tenant-safe message history search on:
  - `POST /instance/:id/messages/search`
  - `POST /chat/findMessages/:instanceName`
- added legacy compatibility routes for the current frontend chat composer:
  - `POST /message/sendText/:instanceName`
  - `POST /message/sendMedia/:instanceName`
  - `POST /message/sendWhatsAppAudio/:instanceName`
  - `POST /chat/findChats/:instanceName`
- introduced a persisted `ConversationMessage` read model scoped by tenant, instance, and `remoteJid`
- persisted outbound text, media, and audio sends into that read model
- wired inbound runtime message events and delivery receipts into the same read model where the active bridge can safely provide them
- added an inbound webhook fallback path that also publishes into the conversation history registry when enough message metadata is present
- added history-sync ingestion from the bridge so replayed WhatsApp messages are persisted into the SaaS conversation history model
- added a tenant-safe backfill trigger on:
  - `POST /instance/:id/history/backfill`
  - plus `/instance/id/:instanceID/history/backfill`
- backfill requests can use:
  - an explicit anchor (`chat_jid`, `message_id`, `timestamp`)
  - or the latest already-persisted message for that chat as a derived anchor

### Connector work already completed

- implemented tenant-safe instance routes for:
  - websocket
  - rabbitmq
  - proxy
- kept unsupported suites explicit as `501 partial` instead of pretending parity

### MVP hardening pass completed

- standardized supported-route error responses around `{ error, message, code }`
- added shared validation-error envelopes for auth, tenant, CRM, broadcast, AI, and instance runtime handlers
- hardened tenant create input normalization and minimum admin password validation
- hardened AI tenant settings to the currently supported `openai`-compatible provider surface
- hardened CRM phone validation so empty digit-only payloads fail fast instead of creating ambiguous contacts
- hardened runtime/backfill input parsing so malformed timestamps fail honestly instead of being silently ignored
- added clearer operator-facing runtime action and observability fields:
  - `operator_message`
  - `bridge_dependent`
  - `status_refresh`
- clamped broadcast list limits and rejected negative broadcast pacing/retry values
- added a broadcast queue log entry with tenant/instance context for operator troubleshooting
- replaced the broadcast noop processor with real WhatsApp text delivery through the tenant-safe instance send path
- broadcast recipient resolution now comes from tenant CRM contacts scoped to the chosen instance or left unscoped for the tenant
- broadcast jobs now fail permanently after partial delivery instead of retrying and risking duplicate sends without recipient-level checkpoints

## Why these changes were made

- to move the fork toward practical Evolution Go / Manager parity without reviving unsafe global legacy routes
- to satisfy the current sibling frontend’s real dependencies first
- to preserve tenant safety and current SaaS architecture
- to keep unsupported surfaces explicit instead of silently failing or fake-completing them

## Important remaining gaps

- inbound history is still partial because there is no backfill from older sessions or full upstream history replay
- Chatwoot, SQS, and manager-style integration suites remain explicit `501 partial`
- dashboard metrics still include placeholders
- runtime parity still depends heavily on the legacy bridge for live snapshots, QR retrieval, and connection actions
- logout truthfulness is limited by the live bridge: if there is no active logged-in runtime session, the backend now returns an explicit error instead of faking success
- durable runtime state is only as complete as the events this SaaS process has observed since the feature was introduced
- history replay improves inbound completeness, but it cannot reconstruct a complete older connect/disconnect/logout timeline from the bridge
- replayed media payloads do not imply durable SaaS media storage; backfill currently persists metadata and structured message bodies only
- broadcast still lacks recipient-level progress persistence and aggregate delivery analytics, so partial deliveries currently fail closed rather than resume mid-audience
- some large multi-package `go test` runs can still hit Windows linker memory limits in this environment, so targeted package verification is more reliable than one giant test invocation

## Files changed in this wave

High-signal files updated for this phase include:

- `internal/ai/handler.go`
- `internal/ai/service.go`
- `internal/auth/handler.go`
- `internal/broadcast/handler.go`
- `internal/broadcast/service.go`
- `internal/crm/handler.go`
- `internal/crm/service.go`
- `internal/handler/http.go`
- `internal/instance/chat_media_types.go`
- `internal/instance/backfill_test.go`
- `internal/instance/compat_handler.go`
- `internal/instance/integration_handler.go`
- `internal/instance/runtime.go`
- `internal/instance/service.go`
- `internal/instance/handler.go`
- `internal/repository/gorm.go`
- `internal/repository/interfaces.go`
- `internal/repository/models.go`
- `internal/server/server.go`
- `internal/service/app.go`
- `internal/tenant/handler.go`
- `internal/tenant/service.go`
- `internal/webhook/service.go`
- `internal/webhook/service_test.go`
- `migrations/000001_saas_core.sql`
- `pkg/runtimeobs/registry.go`
- `pkg/whatsmeow/service/whatsmeow.go`
- `docs/backend-api.md`
- `docs/backend-parity-report.md`
- `docs/backend-parity-plan.md`
- `docs/backend-product-readiness.md`
- `docs/backend-worklog.md`
- `CHANGELOG.md`

## Consistency notes

### Now more aligned

- backend docs reflect that media and audio sending are no longer `501`
- backend docs now call out chat-list parity versus message-history persistence separately
- current frontend chat send routes now have explicit backend compatibility routes instead of only SaaS instance routes
- current frontend chat history pages now have a truthful tenant-safe `Message[]` search surface

### Still intentionally partial

- inbound history completeness and backfill across older sessions
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
- `go build -o api2.exe ./cmd/api` passed
- `go test ./internal/instance ./internal/broadcast ./pkg/sendstatus` passed
- the MVP hardening pass is additionally verified with targeted package tests and a fresh `go build -o api.exe ./cmd/api`
