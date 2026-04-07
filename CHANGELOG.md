# Changelog

## Unreleased

### Backend architecture

- Added a new SaaS API entry point under `cmd/api`
- Added a new `internal/` application layer for auth, tenancy, instances, CRM, broadcast, webhooks, AI, middleware, repository, bootstrap, and server wiring
- Added `migrations/000001_saas_core.sql` as a SQL baseline for the SaaS schema
- Added structured JSON logging helper in `pkg/logger/structured.go`

### Multi-tenancy and auth

- Added tenant, user, and tenant-scoped repository models
- Added stateless JWT access and refresh token handling
- Added tenant API key hashing and verification
- Added RBAC role checks for `owner`, `admin`, and `agent`
- Added tenant middleware to enforce authenticated tenant context and optional tenant header matching
- Added fallback auth that accepts legacy instance tokens and maps them to the owning tenant identity
- Hardened supported MVP auth responses so `/auth/me` returns both `api_key` and `api_key_auth`, and `/auth/logout` returns an explicit `accepted` acknowledgement

### SaaS domain modules

- Added CRM contacts, tags, and notes modules
- Added broadcast queueing, worker claiming, retry scheduling, and rate pacing
- Added tenant webhook endpoint registry and outbound/inbound dispatch
- Added AI tenant settings, instance toggles, conversation memory, queued processing, and outbound webhook emission for generated replies
- Added dashboard metrics endpoint with real instance counts and placeholder aggregates for other totals

### Legacy runtime bridge and compatibility

- Added a runtime bridge from the SaaS API into the legacy instance/WhatsApp engine
- Added normalized QR and status response envelopes for frontend compatibility
- Added compatibility for legacy webhook payloads and instance-name-based webhook updates
- Added compatibility aliases for instance token/api key fields in instance responses
- Added advanced settings bridge routes so frontend toggles can persist `ignoreGroups`, `ignoreStatus`, and related legacy instance flags
- Added guard logic to avoid duplicate concurrent instance startup attempts
- Added safer media-storage fallback behavior when MinIO is enabled but runtime storage is not initialized
- Added stable text delivery tracking with queued job status plus delivered/read receipt updates
- Added tenant-safe media and audio send routes on `/instance/:id/messages/media` and `/instance/:id/messages/audio`
- Added a tenant-safe `ConversationMessage` history model for chat search parity
- Added message-history search on:
  - `/instance/:id/messages/search`
  - `/chat/findMessages/:instanceName`
- Added bridge callbacks to persist inbound runtime messages and delivery/read receipts into the SaaS history model
- Added legacy compatibility send routes for the current frontend:
  - `/message/sendText/:instanceName`
  - `/message/sendMedia/:instanceName`
  - `/message/sendWhatsAppAudio/:instanceName`
- Added live runtime-backed chat list support on:
  - `/instance/:id/chats/search`
  - `/chat/findChats/:instanceName`
- Added tenant-safe runtime admin routes for:
  - `/instance/:id/reconnect`
  - `/instance/:id/pair`
  - `/instance/:id/logout`
  - plus `/instance/id/:instanceID/*` aliases
- Added compatibility-envelope runtime action responses so reconnect/pair/logout can double as frontend status refresh payloads
- Kept logout honest to bridge guarantees by returning an error when no active logged-in runtime session exists instead of faking deeper parity
- Added durable runtime observability with persisted per-instance runtime state plus lifecycle event history
- Added tenant-safe runtime observability routes on:
  - `/instance/:id/runtime`
  - `/instance/:id/runtime/history`
  - plus `/instance/id/:instanceID/*` aliases
- Added normalized lifecycle persistence for `connected`, `disconnected`, `pairing_started`, `paired`, `reconnect_requested`, `logout`, and `status_observed`
- Added replay/backfill persistence for bridge-delivered WhatsApp `HistorySync` blobs
- Added tenant-safe history backfill trigger routes on:
  - `/instance/:id/history/backfill`
  - plus `/instance/id/:instanceID/history/backfill`
- Added safe anchor resolution for backfill requests using either an explicit message anchor or the latest already-persisted message for that tenant-scoped chat
- Added runtime replay checkpoints `history_sync_requested` and `history_sync` to the durable runtime history model
- Added bridge lifecycle publishers from WhatsMeow into the SaaS runtime observability model
- Added inbound webhook fallback publishing into the chat-history registry to improve inbound persistence reliability when webhook dispatch carries enough message metadata
- Hardened supported MVP runtime/chat responses with normalized `{ error, message, code }` error DTOs and clearer operator-facing runtime/backfill envelopes
- Hardened history backfill validation so malformed timestamps fail explicitly instead of being silently ignored

### Configuration and examples

- Replaced stale root `.env.example` with a current example covering both the SaaS API config and the legacy runtime bridge
- Updated `docker/examples/.env.example` to reflect the current runtime variables
- Ignored generated local runtime artifacts such as `api.exe`, `api.pid`, and `api*.log`

### Known partial areas

- Broadcast delivery is still a processor stub and is not yet wired to WhatsApp sending
- Chat list remains live-bridge-backed and can still inherit upstream rate limits
- Redis rate limiting is not implemented yet
- Swagger artifacts under `docs/` still represent older/legacy API surfaces and remain out of sync with `cmd/api`
- The SaaS API still depends on the legacy engine for QR, connection lifecycle, and advanced instance settings
- Added `docs/backend-product-readiness.md` as a practical backend readiness snapshot
- Added `docs/backend-parity-report.md` and `docs/backend-parity-plan.md` for phased parity work
- Message-history parity is now usable, but inbound completeness is still partial because there is no backfill from older sessions or full upstream replay into the SaaS read model
- Durable runtime status/history reads no longer require the live bridge, but live snapshots and connection actions still do
- Replay/backfill improves inbound message completeness, but it still cannot reconstruct a full older connection/logout timeline from the bridge alone

### MVP hardening

- Standardized supported-route validation and error responses around `{ error, message, code }`
- Hardened tenant create validation with trimmed inputs and a minimum admin password length
- Hardened CRM phone validation to reject values that normalize to no digits
- Hardened tenant AI settings to the currently supported `openai`-compatible provider surface
- Hardened broadcast validation by rejecting negative pacing/retry values and clamping list limits
- Added broadcast queue logging with tenant and instance context for operator troubleshooting
