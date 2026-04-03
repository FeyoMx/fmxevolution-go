# Backend Product Readiness

Audited on 2026-04-02.

This summary reflects the current backend mounted by `cmd/api` and `internal/server/server.go`. The backend is product-usable for tenant auth, tenant-scoped instance management, CRM contacts, webhook management, and a subset of instance integrations. It is not yet a fully standalone WhatsApp platform because several capabilities still depend on the legacy runtime bridge in `pkg/*`, and many legacy integration surfaces are intentionally registered as `501 partial`.

## Overall Readiness

Current backend maturity: partial but usable.

Strong areas:

- tenant auth and tenancy enforcement
- tenant-scoped instance CRUD and runtime lifecycle
- webhook endpoint management and dispatch
- CRM contacts/tags/notes
- basic AI settings and event-driven AI reply generation

Main gaps:

- many legacy integration suites are explicit partials
- broadcast and AI do not directly send WhatsApp messages
- some metrics and infra abstractions are placeholders
- the SaaS API still depends on the legacy engine for core real-time instance behavior

## Fully Implemented Routes and Features

### Public and identity

Implemented routes:

- `GET /healthz`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /tenant`
- `GET /auth/me`
- `POST /auth/logout`
- `GET /tenant`

Readiness notes:

- JWT access and refresh token flow is implemented.
- Tenant API key auth is implemented.
- Tenant middleware and role checks are implemented.
- Legacy instance token fallback auth is implemented for compatibility.

### Dashboard

Implemented route:

- `GET /dashboard/metrics`

Readiness notes:

- Route is live and returns instance counts correctly.
- Several non-instance counters are placeholders, so the route is implemented but only partially trustworthy as a product analytics surface.

### AI

Implemented routes:

- `GET /ai/settings`
- `PUT /ai/settings`
- `GET /ai/instances/:instanceID`
- `PUT /ai/instances/:instanceID`

Readiness notes:

- Tenant AI config storage is implemented.
- Per-instance AI enablement and auto-reply flags are implemented.
- Inbound webhook events can enqueue AI processing.
- Conversation memory is stored.
- OpenAI-compatible completion calls are implemented.
- Generated replies are emitted as outbound webhook events.

### Instances

Implemented routes:

- `POST /instance`
- `GET /instance`
- `GET /instance/:id`
- `GET /instance/id/:instanceID`
- `GET /instance/:id/settings`
- `DELETE /instance`
- `DELETE /instance/:id`
- `DELETE /instance/id/:instanceID`
- `POST /instance/:id/connect`
- `POST /instance/id/:instanceID/connect`
- `POST /instance/:id/disconnect`
- `POST /instance/id/:instanceID/disconnect`
- `GET /instance/:id/status`
- `GET /instance/id/:instanceID/status`
- `GET /instance/:id/qr`
- `GET /instance/:id/qrcode`
- `GET /instance/id/:instanceID/qr`
- `GET /instance/id/:instanceID/qrcode`
- `GET /instance/:id/advanced-settings`
- `PUT /instance/:id/advanced-settings`
- `GET /instance/id/:instanceID/advanced-settings`
- `PUT /instance/id/:instanceID/advanced-settings`
- `POST /instance/:id/messages/text`
- `POST /instance/id/:instanceID/messages/text`

Readiness notes:

- Tenant-scoped instance CRUD is implemented.
- Connect, disconnect, status, and QR flows are implemented through the legacy runtime bridge.
- Advanced settings are implemented through the legacy bridge.
- Text sending is implemented.
- Compatibility response shaping for older frontend consumers is implemented.

### Instance event connectors and proxy

Implemented routes:

- `GET /instance/:id/websocket`
- `PUT /instance/:id/websocket`
- `GET /instance/:id/rabbitmq`
- `PUT /instance/:id/rabbitmq`
- `GET /instance/:id/proxy`
- `PUT /instance/:id/proxy`

Readiness notes:

- Websocket and RabbitMQ config routes are implemented through the legacy bridge.
- Proxy config is implemented.
- Proxy support is currently limited to `socks5`.

### CRM

Implemented routes:

- `GET /contacts`
- `GET /contacts/:id`
- `POST /contacts`
- `PATCH /contacts/:id`
- `POST /contacts/:id/notes`
- `POST /contacts/:id/tags`

Readiness notes:

- Contact CRUD-lite is implemented.
- Tags and notes are implemented.
- Tenant scoping and duplicate phone protection are implemented.

### Broadcast

Implemented routes:

- `GET /broadcast`
- `POST /broadcast`
- `GET /broadcast/:id`

Readiness notes:

- Job creation, tenant scoping, listing, storage, worker loop, retry handling, and pacing are implemented.
- Product outcome is still partial because the processor is a stub and does not perform WhatsApp delivery.

### Webhooks

Implemented routes:

- `GET /webhook`
- `POST /webhook`
- `GET /webhook/:id`
- `POST /webhook/inbound`
- `POST /webhook/outbound`

Readiness notes:

- Tenant-managed webhook endpoint registry is implemented.
- Dispatch is implemented.
- Legacy-compatible webhook payload parsing is implemented.
- Inbound webhook dispatch can trigger the AI pipeline.

## Partial and `501` Routes

These routes are intentionally mounted and return structured `501 partial` responses instead of pretending to work.

### Partial chat surface

- `POST /instance/:id/chats/search`
- `POST /instance/:id/messages/search`
- `POST /instance/:id/messages/media`
- `POST /instance/:id/messages/audio`

Current blockers:

- no tenant-safe SaaS chat/message repository matching the legacy frontend contracts
- media and audio sending not yet wired into the SaaS instance service
- only text sending is currently supported

### Partial connector surface

- `GET /instance/:id/sqs`
- `PUT /instance/:id/sqs`
- `GET /instance/:id/chatwoot`
- `PUT /instance/:id/chatwoot`

Current blockers:

- no tenant-safe repository/runtime support in the SaaS layer

### Partial integration suites

All of the following are intentionally unsupported today and return `501 partial`:

- `GET/POST /instance/:id/openai`
- `GET/PUT/DELETE /instance/:id/openai/:resourceId`
- `GET/PUT /instance/:id/openai/settings`
- `GET /instance/:id/openai/:resourceId/sessions`
- `POST /instance/:id/openai/status`
- `GET/POST /instance/:id/typebot`
- `GET/PUT/DELETE /instance/:id/typebot/:resourceId`
- `GET/PUT /instance/:id/typebot/settings`
- `GET /instance/:id/typebot/:resourceId/sessions`
- `POST /instance/:id/typebot/status`
- `GET/POST /instance/:id/dify`
- `GET/PUT/DELETE /instance/:id/dify/:resourceId`
- `GET/PUT /instance/:id/dify/settings`
- `GET /instance/:id/dify/:resourceId/sessions`
- `POST /instance/:id/dify/status`
- `GET/POST /instance/:id/n8n`
- `GET/PUT/DELETE /instance/:id/n8n/:resourceId`
- `GET/PUT /instance/:id/n8n/settings`
- `GET /instance/:id/n8n/:resourceId/sessions`
- `POST /instance/:id/n8n/status`
- `GET/POST /instance/:id/evoai`
- `GET/PUT/DELETE /instance/:id/evoai/:resourceId`
- `GET/PUT /instance/:id/evoai/settings`
- `GET /instance/:id/evoai/:resourceId/sessions`
- `POST /instance/:id/evoai/status`
- `GET/POST /instance/:id/evolutionBot`
- `GET/PUT/DELETE /instance/:id/evolutionBot/:resourceId`
- `GET/PUT /instance/:id/evolutionBot/settings`
- `GET /instance/:id/evolutionBot/:resourceId/sessions`
- `POST /instance/:id/evolutionBot/status`
- `GET/POST /instance/:id/flowise`
- `GET/PUT/DELETE /instance/:id/flowise/:resourceId`
- `GET/PUT /instance/:id/flowise/settings`
- `GET /instance/:id/flowise/:resourceId/sessions`
- `POST /instance/:id/flowise/status`

Shared blocker:

- the current backend does not have tenant-safe repository and runtime support for these legacy integration models

## Unsupported Areas

The following areas are effectively unsupported from a product perspective, even when related route names exist:

- direct WhatsApp sending for AI-generated replies
- direct WhatsApp broadcast delivery
- tenant-safe legacy chat history and message search
- media and audio sending from the SaaS instance routes
- Chatwoot, SQS, OpenAI resource CRUD, Typebot, Dify, N8N, EvoAI, Evolution Bot, and Flowise integration management
- tenant quotas, billing, and usage accounting
- persistent metrics aggregation beyond simple live instance counts
- Redis-backed rate limiting
- standalone SaaS runtime independence from the legacy engine

Also unsupported or stale around developer/product operations:

- generated Swagger docs under `docs/` do not match the mounted `cmd/api` routes
- SQL migrations exist as reference, but startup still relies on GORM auto-migration rather than a real migration runner

## Known Technical Debt

- Hybrid architecture debt: the new SaaS API still depends on `pkg/*` legacy runtime code for connect, disconnect, QR, status, webhook sync, and advanced settings.
- Runtime bridge coupling: if the legacy runtime cannot initialize, important instance lifecycle features degrade or become unavailable.
- Data model split: advanced settings live in the legacy instance model instead of the SaaS `instances` table.
- Broadcast debt: the queue, retries, and workers exist, but delivery uses a no-op processor stub.
- AI debt: AI generation works, but the result stops at webhook emission instead of sending a WhatsApp reply.
- Metrics debt: dashboard metrics mix real instance counts with placeholder zeros.
- Rate-limit backend debt: `redis` is only a placeholder and currently falls back to memory behavior.
- Auth/session debt: refresh tokens are stateless and not revocable; logout is only an acknowledgement response.
- Migration debt: `gorm.AutoMigrate` is the real startup path, while `migrations/000001_saas_core.sql` is not executed automatically.
- Documentation debt: stale swagger artifacts can mislead frontend and QA work.
- Compatibility debt: several response shims and legacy token aliases are still required to keep older clients working.

## Next Recommended Backend Priorities

1. Replace the broadcast processor stub with real WhatsApp delivery through the instance runtime.
2. Finish AI auto-reply by sending generated replies through the runtime instead of only emitting webhook events.
3. Decide which instance integrations are true product priorities and either implement tenant-safe support or remove/deprecate the `501` surfaces from active product scope.
4. Implement tenant-safe media and audio sending, then decide whether chat/message search belongs in SaaS storage or should remain explicitly unsupported.
5. Reduce dependency on the legacy runtime by moving more instance state and settings into the SaaS-owned data model.
6. Replace `AutoMigrate`-only startup with an actual migration workflow and make schema evolution deterministic.
7. Refresh or regenerate Swagger/OpenAPI docs from the mounted `cmd/api` routes.
8. Implement real metrics aggregation so dashboard counters are product-credible.
9. Add a real Redis-backed rate-limit store if multi-instance deployment is a target.
10. Add refresh-token revocation/session tracking if security and admin controls matter for production rollout.

## Product Conclusion

The backend is ready for limited SaaS workflows today:

- tenant signup and login
- tenant-scoped instance administration
- connect/QR/status flows
- text sending
- webhook management
- CRM contact management
- early AI and broadcast orchestration

It is not yet ready to claim full parity with the legacy platform or to market the full integration catalog as supported. The biggest product risks are the number of intentional `501` surfaces, placeholder metrics, and the continued dependence on the legacy runtime for core instance behavior.
