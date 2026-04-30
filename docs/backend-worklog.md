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
- added an operator-safe cache/throttle layer around live chat-list queries:
  - cache key is tenant ID + instance ID + normalized chat filter
  - fresh cache TTL is 30 seconds
  - stale fallback window is 5 minutes
  - repeated identical live refreshes are throttled for 5 seconds when cached data exists
  - cache/stale truth is exposed through `X-Evolution-Chat-*` headers while preserving the `Chat[]` body
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
- broadcast recipient progress is now durably persisted per job and per phone with status, attempt counts, last error, timestamps, and send references
- broadcast jobs now seed a recipient snapshot from tenant CRM contacts so the audience is stable across retries
- retryable broadcast failures now pause and resume from pending recipients instead of replaying recipients already marked sent
- permanent recipient failures are tracked per recipient and no longer force the whole job to stop if the rest of the audience can still be processed
- broadcast jobs can now finish as `completed_with_failures` when all recipients are terminal but some failed permanently
- broadcast processing logs now include claim, per-job, and per-recipient attempt/failure details with attempt counters
- broadcast success now requires a confirmed send result from the instance send path instead of treating an empty result as delivered
- broadcast detail now has a dedicated tenant-safe `GET /broadcast/:id/recipients` endpoint with bounded pagination, status filters, and optional phone/contact search
- recipient progress listing returns whole-broadcast durable summary counts plus paginated operator-facing recipient rows for large campaign inspection
- recipient detail remains truthful by exposing only durable progress states (`pending`, `sent`, `delivered`, `read`, `failed`) and by marking old pre-progress broadcasts as partial when needed
- runtime delivery/read receipts are now wired back into broadcast recipient progress when the SaaS layer can safely match `instance_id + message_id` to a stored recipient row
- recipient progress can now advance from `sent` to `delivered` and `read`, and stores `delivered_at`, `read_at`, `last_status_at`, and `status_source` without regressing retry/resume safety
- dashboard recipient aggregates now expose durable `delivered` and `read` counts in addition to sent/failed/pending totals
- dashboard metrics now use stored tenant data for `contacts_total`, `broadcast_total`, and `messages_total`
- dashboard metrics now also expose broadcast recipient totals, attempted, sent, failed, pending, and a partial flag for older untracked jobs
- dashboard runtime metrics now expose `runtime_healthy`, `runtime_degraded`, `runtime_unavailable`, `runtime_unknown`, and `runtime_health_partial`
- `messages_total` is explicitly marked partial because it reflects the tenant-scoped SaaS message-history store, not universal WhatsApp history
- dashboard tenant/user platform totals are now marked unsupported instead of returning placeholder-looking values from a tenant-scoped endpoint
- dashboard responses include `metrics_limitations` for explicit operator-facing caveats
- supported MVP handlers now use English validation messages while preserving the shared `{ error, message, code }` envelope
- broadcast recipient listing now rejects negative pagination values and keeps documented defaults for omitted page/limit
- chat-list bridge failure logs include request ID when present, in addition to tenant and instance context
- lifecycle, backfill, and runtime snapshot failure paths now emit more operator-useful logs with tenant/instance context
- repo-root temp utilities now use `//go:build ignore`, which removes them as blockers for `go test ./...`

### QA data tooling

- added `cmd/qa-seed` plus isolated `internal/qaseed` development tooling
- QA seeding is disabled by default and requires `QA_SEED_ENABLED=true`
- QA seeding refuses `APP_ENV=production` and `APP_ENV=prod`
- the seed path is explicit CLI tooling only and is not wired into API startup
- fixture data is tenant-scoped and deterministic so it can be rerun safely for local/manual QA
- fixture coverage includes:
  - 125 contacts
  - dense and sparse instances
  - mixed-status broadcast jobs and recipient progress rows
  - dense conversation history across multiple chats
  - runtime state plus lifecycle/history runtime events

## Why these changes were made

- to move the fork toward practical Evolution Go / Manager parity without reviving unsafe global legacy routes
- to satisfy the current sibling frontend’s real dependencies first
- to preserve tenant safety and current SaaS architecture
- to keep unsupported surfaces explicit instead of silently failing or fake-completing them

## Important remaining gaps

- inbound history is still partial because there is no backfill from older sessions or full upstream history replay
- chat-list cache reduces bridge pressure but does not create a durable chat metadata model
- Chatwoot, SQS, and manager-style integration suites remain explicit `501 partial`
- runtime parity still depends heavily on the legacy bridge for live snapshots, QR retrieval, and connection actions
- logout truthfulness is limited by the live bridge: if there is no active logged-in runtime session, the backend now returns an explicit error instead of faking success
- durable runtime state is only as complete as the events this SaaS process has observed since the feature was introduced
- history replay improves inbound completeness, but it cannot reconstruct a complete older connect/disconnect/logout timeline from the bridge
- replayed media payloads do not imply durable SaaS media storage; backfill currently persists metadata and structured message bodies only
- some large multi-package `go test` runs can still hit Windows linker memory limits in this environment, so targeted package verification is more reliable than one giant test invocation
- repo-wide `go test ./...` is still blocked by legacy `github.com/chai2010/webp` build failures outside the SaaS sprint slice
- broadcast recipient detail still does not join enriched CRM display names
- richer receipt progression is still best-effort and runtime-dependent; recipients remain at `sent` when later receipt events are absent or cannot be safely matched
- platform-wide tenant/user dashboard totals remain unsupported by the current tenant-scoped metrics endpoint
- QA seed data is intentionally synthetic and development-only; it must not be used to imply production support for unsupported integrations or full durable WhatsApp chat parity

## Files changed in this wave

High-signal files updated for this phase include:

- `internal/ai/handler.go`
- `internal/ai/service.go`
- `internal/auth/handler.go`
- `internal/broadcast/handler.go`
- `internal/broadcast/processor.go`
- `internal/broadcast/service.go`
- `internal/broadcast/service_test.go`
- `internal/dashboard/handler.go`
- `internal/dashboard/service.go`
- `internal/dashboard/service_test.go`
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
- `internal/qaseed/seed.go`
- `cmd/qa-seed/main.go`
- `internal/webhook/service.go`
- `internal/webhook/service_test.go`
- `migrations/000001_saas_core.sql`
- `tmp_fix_remote_jid.go`
- `tmp_schema_check.go`
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
- chat-list metadata parity; cached results are temporary snapshots, not a persisted chat table
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
- `go test ./internal/broadcast ./internal/dashboard ./internal/server ./internal/crm ./internal/instance` passed
- `go build -mod=readonly ./cmd/api` passed
- `go test ./...` was attempted and now clears the repo-root temp-file blocker, but it still fails in legacy packages because `github.com/chai2010/webp` does not build in this environment
