# Backend Product Readiness

Audited on 2026-04-06.

This summary reflects the backend mounted by `cmd/api` and `internal/server/server.go`, compared against the bundled upstream-style legacy surface still present under `pkg/*` and the current sibling frontend repo. The backend is product-usable for tenant auth, tenant-scoped instance lifecycle, webhook dispatch, CRM, text sending, media sending, audio sending, a live chat list, and tenant-safe message-history search. It is not yet full Evolution Go / Manager parity because several manager integration pages, some runtime/admin surfaces, and full upstream chat-history replay remain partial or unsupported.

## Overall Readiness

Current backend maturity: usable, with explicit partials.

Strong areas:

- tenant auth, JWT, API key, and legacy instance-token compatibility
- tenant-scoped instance CRUD, connect, disconnect, reconnect, pair, logout, QR, status, and advanced settings
- text sending with async delivery tracking
- media and audio sending through the legacy runtime bridge
- webhook management and tenant-safe dispatch
- websocket, rabbitmq, and proxy instance settings
- CRM contacts/tags/notes

Main gaps:

- manager-style integration suites still return explicit `501 partial`
- dashboard metrics still contain placeholder counters
- runtime parity still depends on the legacy bridge in `pkg/*`
- inbound history persistence is still bridge-dependent and not backfilled from older sessions

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

- JWT access and refresh flow is implemented.
- Tenant API key auth is implemented.
- Legacy instance token fallback auth is implemented.
- Tenant scoping is enforced by auth plus tenant middleware.

### Dashboard

Implemented route:

- `GET /dashboard/metrics`

Readiness notes:

- Instance counters are real.
- Several other counters are placeholders, so this route is implemented but not yet full analytics parity.

### AI

Implemented routes:

- `GET /ai/settings`
- `PUT /ai/settings`
- `GET /ai/instances/:instanceID`
- `PUT /ai/instances/:instanceID`

Readiness notes:

- Tenant AI settings and per-instance toggles are implemented.
- AI replies are generated and emitted through outbound webhook events.
- This is still weaker than full manager/runtime bot parity because this SaaS layer does not own all legacy bot CRUD suites.

### Instances and runtime lifecycle

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
- `POST /instance/:id/reconnect`
- `POST /instance/id/:instanceID/reconnect`
- `POST /instance/:id/pair`
- `POST /instance/id/:instanceID/pair`
- `DELETE /instance/:id/logout`
- `DELETE /instance/id/:instanceID/logout`
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

Readiness notes:

- Tenant-scoped instance CRUD is implemented.
- Connect, disconnect, reconnect, pair, logout, QR, and status are implemented through the legacy runtime bridge.
- Advanced settings are implemented through the legacy bridge.
- Response shaping keeps older frontend consumers working.
- Runtime admin actions stay tenant-safe because the SaaS layer resolves the instance inside the authenticated tenant before invoking the bridge.
- Logout remains intentionally stricter than legacy-global behavior: the bridge only reports success when there is an active logged-in runtime session to terminate.

### Messaging

Implemented SaaS routes:

- `POST /instance/:id/messages/text`
- `GET /instance/:id/messages/text/:jobID`
- `POST /instance/id/:instanceID/messages/text`
- `GET /instance/id/:instanceID/messages/text/:jobID`
- `POST /instance/:id/messages/media`
- `POST /instance/:id/messages/audio`
- `POST /instance/:id/chats/search`
- `POST /instance/:id/messages/search`

Implemented compatibility routes:

- `POST /message/sendText/:instanceName`
- `POST /message/sendMedia/:instanceName`
- `POST /message/sendWhatsAppAudio/:instanceName`
- `POST /chat/findChats/:instanceName`
- `POST /chat/findMessages/:instanceName`

Readiness notes:

- Text sending is implemented and job status now tracks `queued`, `running`, `sent`, `delivered`, and `read`.
- Media sending is implemented through the legacy runtime bridge using JSON base64 payloads or URL payloads.
- Audio sending is implemented through the legacy runtime bridge and audio conversion path.
- Chat search is functional as a live runtime-backed list of contacts and groups.
- Message-history search is implemented against a tenant-safe `ConversationMessage` read model scoped by tenant, instance, and `remoteJid`.
- Outbound text, media, and audio sends are persisted into that read model.
- Inbound runtime events are persisted when the active legacy bridge delivers them into the current process.
- Chat list parity is still partial because the backend does not persist legacy chat metadata such as full labels, last-message previews, or conversation ordering from durable storage.

### Instance event connectors and proxy

Implemented routes:

- `GET /instance/:id/websocket`
- `PUT /instance/:id/websocket`
- `GET /instance/:id/rabbitmq`
- `PUT /instance/:id/rabbitmq`
- `GET /instance/:id/proxy`
- `PUT /instance/:id/proxy`

Readiness notes:

- Websocket and RabbitMQ config are bridged to the legacy runtime model.
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

### Broadcast

Implemented routes:

- `GET /broadcast`
- `POST /broadcast`
- `GET /broadcast/:id`

Readiness notes:

- Queueing, tenant scoping, and worker claiming are implemented.
- Delivery execution is still partial because WhatsApp send-out is still a stub.

### Webhooks

Implemented routes:

- `GET /webhook`
- `POST /webhook`
- `GET /webhook/:id`
- `POST /webhook/inbound`
- `POST /webhook/outbound`

Readiness notes:

- Tenant-managed webhook endpoint registry is implemented.
- Dispatch and inbound AI trigger flow are implemented.

## Partially Matched Features

These areas are functional enough to be mounted, but they are not full upstream parity yet.

### Chat list parity

Functional routes:

- `POST /instance/:id/chats/search`
- `POST /chat/findChats/:instanceName`

Current limitations:

- live runtime contacts/groups only
- no tenant-safe persisted chat table
- no true last-message preview parity
- labels are not sourced from a SaaS chat repository

### Dashboard analytics parity

Functional route:

- `GET /dashboard/metrics`

Current limitations:

- only instance counters are trustworthy
- contact/message/revenue-style analytics are not implemented

### AI parity

Functional routes exist for tenant and instance AI settings, but these do not replace the legacy manager’s OpenAI/Typebot/Dify/N8N/EvoAI/EvolutionBot/Flowise CRUD suites.

## Explicit Partial and `501` Routes

These routes are intentionally mounted and still return structured `501 partial` responses.

### Connector and manager integration surfaces

- `GET /instance/:id/sqs`
- `PUT /instance/:id/sqs`
- `GET /instance/:id/chatwoot`
- `PUT /instance/:id/chatwoot`
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

## Unsupported or Still Missing Areas

These capabilities are still missing from the active SaaS surface even though they exist upstream or in legacy manager expectations:

- Kafka connector parity
- tenant-safe Chatwoot storage and runtime wiring
- tenant-safe CRUD for OpenAI, Typebot, Dify, N8N, EvoAI, EvolutionBot, and Flowise
- full upstream chat-history replay/backfill parity
- full broadcast-to-WhatsApp execution
- trustworthy product analytics beyond instance counts

## Known Technical Debt

- core runtime behavior still depends on the legacy bridge in `pkg/*`
- message history now uses a tenant-safe SaaS repository, but inbound capture still depends on active bridge events and receipts
- many manager integration suites are represented only as explicit `501` placeholders
- dashboard metrics still contain placeholders
- Swagger artifacts remain stale relative to `cmd/api`
- current media/audio implementation is intentionally scoped to JSON payloads used by the current frontend, not every upstream transport shape

## Next Recommended Backend Priorities

1. Add durable inbound/backfill parity so message history is not limited to events seen by the current SaaS process.
2. Add durable runtime session observability so reconnect/logout/pair UX can report richer state without relying entirely on the live legacy bridge.
3. Decide which manager integration suites are true product priorities and implement only those with tenant-safe repositories.
4. Replace placeholder dashboard metrics with real aggregates or label them partial in UI.
5. Reduce remaining reliance on legacy bridge internals by moving reusable runtime adapters into `internal/instance`.
