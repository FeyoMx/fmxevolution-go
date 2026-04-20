# Backend Product Readiness

Audited on 2026-04-06.

This summary reflects the backend mounted by `cmd/api` and `internal/server/server.go`, compared against the bundled upstream-style legacy surface still present under `pkg/*` and the current sibling frontend repo. The backend is product-usable for tenant auth, tenant-scoped instance lifecycle, durable runtime observability, webhook dispatch, CRM, text sending, media sending, audio sending, a live chat list, tenant-safe message-history search, and limited replay/backfill ingestion. It is not yet full Evolution Go / Manager parity because several manager integration pages, some runtime/admin surfaces, and full upstream chat-history replay remain partial or unsupported.

## Overall Readiness

Current backend maturity: usable, with explicit partials.

Strong areas:

- tenant auth, JWT, API key, and legacy instance-token compatibility
- tenant-scoped instance CRUD, connect, disconnect, reconnect, pair, logout, QR, status, advanced settings, and durable runtime history
- text sending with async delivery tracking
- media and audio sending through the legacy runtime bridge
- webhook management and tenant-safe dispatch
- websocket, rabbitmq, and proxy instance settings
- CRM contacts/tags/notes
- shared error DTOs plus clearer operator-facing runtime/backfill responses across the supported MVP routes

Main gaps:

- manager-style integration suites still return explicit `501 partial`
- dashboard metrics are materially better, but message totals and some recipient aggregates remain explicitly partial when historical broadcasts predate recipient progress tracking
- runtime parity still depends on the legacy bridge in `pkg/*`
- inbound history persistence is more reliable, and bridge-delivered history-sync blobs are now ingested, but completeness is still bridge-dependent and not universally backfilled from arbitrary older sessions

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
- `GET /auth/me` now returns both `api_key` and `api_key_auth` for frontend compatibility.
- `POST /auth/logout` is a stateless acknowledgement and now returns `accepted: true`.

### Dashboard

Implemented route:

- `GET /dashboard/metrics`

Readiness notes:

- Instance counters are real.
- `contacts_total` and `broadcast_total` are now counted from stored tenant data.
- `messages_total` is now counted from stored tenant conversation history and explicitly flagged partial.
- Broadcast recipient totals, attempted, sent, failed, and pending are now counted from durable recipient progress rows and explicitly flagged partial when some historical jobs have no recipient snapshot yet.
- Broadcast recipient aggregates now also expose durable `delivered` and `read` counts when runtime receipt events can be safely matched back to recipient progress rows.
- Runtime health counters are now exposed as healthy/degraded/unavailable/unknown buckets with a partial flag when some instances have no durable runtime state yet.

### AI

Implemented routes:

- `GET /ai/settings`
- `PUT /ai/settings`
- `GET /ai/instances/:instanceID`
- `PUT /ai/instances/:instanceID`

Readiness notes:

- Tenant AI settings and per-instance toggles are implemented.
- AI replies are generated and emitted through outbound webhook events.
- Tenant AI settings are intentionally constrained to `openai`-compatible providers in the supported MVP surface.
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
- `GET /instance/:id/runtime`
- `GET /instance/id/:instanceID/runtime`
- `GET /instance/:id/runtime/history`
- `GET /instance/id/:instanceID/runtime/history`
- `POST /instance/:id/history/backfill`
- `POST /instance/id/:instanceID/history/backfill`
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
- Connect, disconnect, reconnect, pair, logout, QR, and live status are implemented through the legacy runtime bridge.
- Durable runtime session state and runtime lifecycle events are persisted in the SaaS database and exposed on tenant-safe runtime routes.
- Advanced settings are implemented through the legacy bridge.
- Response shaping keeps older frontend consumers working.
- Runtime admin actions stay tenant-safe because the SaaS layer resolves the instance inside the authenticated tenant before invoking the bridge.
- Logout remains intentionally stricter than legacy-global behavior: the bridge only reports success when there is an active logged-in runtime session to terminate.
- The durable runtime model records `connected`, `disconnected`, `pairing_started`, `paired`, `reconnect_requested`, `logout`, and `status_observed`.
- Runtime replay/backfill is limited to sync checkpoints: the backend now persists `history_sync_requested` and `history_sync` when the bridge accepts and ingests a history sync blob.
- The runtime status endpoint is partially bridge-independent: durable state reads do not require the live bridge, but the optional `live` block still does.
- Runtime action and observability envelopes now include clearer operator-facing fields so the UI can distinguish durable reads from bridge-dependent work.
- Lifecycle actions, runtime status, runtime history, and history backfill now all follow the same compatibility response pattern with nested `data` plus duplicated top-level fields for frontend refresh flows.
- Bridge-unavailable failures are now more consistent for operators: reconnect, pair, logout, and history backfill report `409 conflict` instead of leaking a generic `500` when the SaaS layer cannot reach the live bridge.

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
- Inbound webhook dispatch now also publishes into the same read model when the webhook payload includes enough message metadata.
- History sync replay blobs delivered by WhatsApp are now ingested into the same read model, and the SaaS layer can request an on-demand history sync when given an explicit or stored message anchor.
- Chat list parity is still partial because the backend does not persist legacy chat metadata such as full labels, last-message previews, or conversation ordering from durable storage.
- Runtime/history/chat validation now fails more honestly for malformed backfill timestamps and other malformed operator payloads.

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

Readiness notes:

- Contact phone values are normalized to digits only.
- Contact create and update now reject phone payloads that normalize to an empty value.

### Broadcast

Implemented routes:

- `GET /broadcast`
- `POST /broadcast`
- `GET /broadcast/:id`
- `GET /broadcast/:id/recipients`

Readiness notes:

- Queueing, tenant scoping, and worker claiming are implemented.
- Broadcast create now rejects negative delay/rate/retry values, and broadcast list clamps `limit` to a bounded range.
- Broadcast jobs now perform real WhatsApp text send attempts through the tenant-safe instance send path.
- The current recipient source is the tenant CRM contact list, limited to contacts with no `instance_id` or a matching `instance_id`, and the recipient set is snapshotted into durable progress rows.
- Jobs fail honestly when there are no eligible contacts or when the target runtime cannot send.
- Jobs also fail honestly when the instance send path returns no confirmable send result for a recipient.
- Retryable failures now resume safely from pending recipients without duplicating recipients already marked `sent`.
- Permanent recipient failures are tracked per recipient and surfaced in job analytics.
- Broadcast jobs can now finish as `completed_with_failures` when the audience is fully processed but some recipients are terminal failures.
- Broadcast detail is now operator-friendly for larger campaigns because recipient progress can be paginated and filtered by `pending`, `sent`, and `failed`.
- Broadcast detail is now operator-friendly for larger campaigns because recipient progress can be paginated and filtered by `pending`, `sent`, `delivered`, `read`, and `failed`.
- Recipient listing returns durable operator-facing fields such as phone, `contact_id`, attempt count, last error, timestamps, send references, and receipt progression metadata when available.
- Broadcast recipient listing stays truthful: it exposes only stored recipient progress and whole-broadcast durable summary counts, without inventing downstream delivery receipt states. `delivered` and `read` are best-effort and appear only when real runtime receipts are safely mapped back by durable identifiers.

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

### Runtime observability parity

Functional routes:

- `GET /instance/:id/runtime`
- `GET /instance/:id/runtime/history`

Current limitations:

- durable state is only as complete as the lifecycle events observed by this SaaS process
- the `live` runtime block still depends on the legacy bridge being reachable
- there is no full historical replay for runtime lifecycle events that happened before observability was added; replay currently adds sync checkpoints, not a reconstructable connection timeline

### Replay / backfill parity

Functional routes:

- `POST /instance/:id/history/backfill`

Current limitations:

- backfill requires either an explicit message anchor or an already-persisted message for that chat so the backend can derive one safely
- the bridge can backfill message history, but not a full older session lifecycle timeline
- replayed media content is not re-uploaded into SaaS storage; only message metadata and structured payloads are persisted

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

- instance, contact, and broadcast totals are trustworthy
- recipient-level broadcast analytics are trustworthy for broadcasts with seeded recipient progress
- message totals are truthful for the SaaS history store but explicitly partial relative to global WhatsApp history
- there is still no revenue or campaign analytics layer

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
- old historical broadcasts created before recipient progress tracking may still show partial recipient analytics
- full product analytics beyond tenant-scoped operational counts

## Known Technical Debt

- core runtime behavior still depends on the legacy bridge in `pkg/*`
- message history now uses a tenant-safe SaaS repository, but inbound capture still depends on active bridge events and receipts
- chat list still depends on live bridge queries and can surface upstream rate-limit behavior
- many manager integration suites are represented only as explicit `501` placeholders
- message totals remain explicitly partial because they only count stored SaaS history rows
- recipient aggregates also remain partially historical until older broadcasts are re-run or backfilled into recipient progress rows
- Swagger artifacts remain stale relative to `cmd/api`
- current media/audio implementation is intentionally scoped to JSON payloads used by the current frontend, not every upstream transport shape

## Next Recommended Backend Priorities

1. Add operator-safe throttling or caching around live chat-list queries so bridge rate limits do not degrade MVP UX.
2. Tighten targeted integration tests around auth, runtime actions, message search, and backfill envelopes.
3. Add operator-facing broadcast recipient exports or cursor-based pagination for very large campaigns if list sizes outgrow the current page model.
4. Reduce remaining reliance on legacy bridge internals by moving reusable runtime adapters into `internal/instance`.
5. Decide which manager integration suites are true product priorities and keep the rest explicitly unsupported.
