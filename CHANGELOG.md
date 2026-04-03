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

### Configuration and examples

- Replaced stale root `.env.example` with a current example covering both the SaaS API config and the legacy runtime bridge
- Updated `docker/examples/.env.example` to reflect the current runtime variables
- Ignored generated local runtime artifacts such as `api.exe`, `api.pid`, and `api*.log`

### Known partial areas

- Broadcast delivery is still a processor stub and is not yet wired to WhatsApp sending
- Redis rate limiting is not implemented yet
- Swagger artifacts under `docs/` still represent older/legacy API surfaces and remain out of sync with `cmd/api`
- The SaaS API still depends on the legacy engine for QR, connection lifecycle, and advanced instance settings
- Added `docs/backend-product-readiness.md` as a practical backend readiness snapshot
