# Backend API

This document describes the routes registered in `internal/server/server.go` for `cmd/api`.

## Common Rules

### Authentication

Public routes:

- `GET /healthz`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /tenant`

Protected routes accept:

- `Authorization: Bearer <access_token>`
- `X-API-Key: <tenant_api_key>`
- `apikey: <tenant_api_key>`

There is also a compatibility fallback that accepts a legacy instance token and maps it to the owning tenant identity.

### Tenant scoping

All protected routes run through:

1. auth middleware
2. tenant middleware

Tenant scope is derived from the authenticated identity, not from arbitrary path values. Optional `X-Tenant-ID` and `X-Tenant-Slug` headers must match the authenticated tenant if present.

### Common error responses

| Status | Meaning |
|---|---|
| `400` | validation or malformed body |
| `401` | missing/invalid auth |
| `403` | role or tenant mismatch |
| `404` | entity not found |
| `409` | uniqueness/conflict |
| `429` | rate limit exceeded |
| `500` | internal error |

Error body shape:

```json
{ "error": "message" }
```

## Public Routes

| Method | Path | Auth | Request body | Success response | Notes |
|---|---|---|---|---|---|
| `GET` | `/healthz` | none | none | `{ "status": "ok" }` | health probe |
| `POST` | `/auth/login` | none | `tenant_slug`, `email`, `password` | `{ access_token, refresh_token, tenant_id, user_id, role, expires_in }` | accepts JSON, form, query, header-assisted tenant slug |
| `POST` | `/auth/refresh` | none | `refresh_token` | same as login | stateless refresh |
| `POST` | `/tenant` | none | `{ name, slug, admin_name, admin_email, admin_password }` | `{ tenant, user, api_key }` | creates tenant plus first admin |

## Auth / Identity

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/auth/me` | owner, admin, agent | none | `{ user_id, tenant_id, email, role, api_key }` | authenticated tenant |
| `POST` | `/auth/logout` | owner, admin, agent | none | `{ "message": "logout exitoso" }` | stateless acknowledgement only |

## Tenant

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/tenant` | owner, admin, agent | none | `repository.Tenant` | current tenant only |

## Dashboard

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/dashboard/metrics` | owner, admin, agent | none | aggregate JSON with instance counts and placeholder totals | current tenant instances only |

Notes:

- `instances_total`, `instances_active`, `instances_inactive` are real
- several other counters are currently placeholder `0`

## AI

### Tenant AI settings

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/ai/settings` | owner, admin, agent | none | `repository.AISettings` | current tenant only |
| `PUT` | `/ai/settings` | owner, admin | `{ enabled, auto_reply, provider, model, base_url, system_prompt }` | `repository.AISettings` | current tenant only |

### Instance AI toggles

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/ai/instances/:instanceID` | owner, admin, agent | none | `{ instance_id, enabled, auto_reply }` | instance must belong to current tenant |
| `PUT` | `/ai/instances/:instanceID` | owner, admin | `{ enabled, auto_reply }` | `{ instance_id, enabled, auto_reply }` | instance must belong to current tenant |

Notes:

- AI reply generation is partial: generated replies are emitted as outbound webhook events, not sent directly to WhatsApp by this SaaS layer.

## Instances

### CRUD and details

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `POST` | `/instance` | owner, admin | `{ name, engine_instance_id?, webhook_url? }` plus legacy aliases like `instanceName`, `instance` | `repository.Instance` | created for current tenant |
| `GET` | `/instance` | owner, admin, agent | none | `[]repository.Instance` | current tenant only |
| `GET` | `/instance/:id` | owner, admin, agent | none | instance detail payload enriched from runtime snapshot when available | current tenant only |
| `GET` | `/instance/id/:instanceID` | owner, admin, agent | none | same as `/instance/:id` | current tenant only |
| `GET` | `/instance/:id/settings` | owner, admin, agent | none | same shape as instance detail payload | current tenant only |
| `DELETE` | `/instance` | owner, admin | query `id` | `204` | current tenant only |
| `DELETE` | `/instance/:id` | owner, admin | none | `204` | current tenant only |
| `DELETE` | `/instance/id/:instanceID` | owner, admin | none | `204` | current tenant only |

Instance detail payload can include:

```json
{
  "id": "uuid",
  "instance_id": "uuid",
  "instanceName": "AstethicBot",
  "name": "AstethicBot",
  "status": "created|connecting|qrcode|open|close|disconnected",
  "engine_instance_id": "uuid-or-bridge-id",
  "webhook_url": "https://...",
  "connected": true,
  "tenant_id": "uuid",
  "apikey": "legacy-instance-token",
  "apiKey": "legacy-instance-token",
  "token": "legacy-instance-token",
  "events": "MESSAGE",
  "ignoreGroups": false,
  "ignoreStatus": false
}
```

### Connect / disconnect / runtime admin

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `POST` | `/instance/:id/connect` | owner, admin | none | `{ message, instance_id, instanceName, status, qrcode?, code?, connected? }` | current tenant only |
| `POST` | `/instance/id/:instanceID/connect` | owner, admin | none | same as above | current tenant only |
| `POST` | `/instance/:id/disconnect` | owner, admin | none | `{ message, instance_id, instanceName, status }` | current tenant only |
| `POST` | `/instance/id/:instanceID/disconnect` | owner, admin | none | same as above | current tenant only |
| `POST` | `/instance/:id/reconnect` | owner, admin | none | compatibility envelope with `{ instance_id, instanceName, engine_instance_id, status, connected, accepted, action: "reconnect" }` | current tenant only |
| `POST` | `/instance/id/:instanceID/reconnect` | owner, admin | none | same as above | current tenant only |
| `POST` | `/instance/:id/pair` | owner, admin | `{ phone }` or `{ number }` | compatibility envelope with `{ instance_id, instanceName, engine_instance_id, status, connected, accepted, action: "pair", code, pairingCode }` | current tenant only |
| `POST` | `/instance/id/:instanceID/pair` | owner, admin | same as above | same as above | current tenant only |
| `DELETE` | `/instance/:id/logout` | owner, admin | none | compatibility envelope with `{ instance_id, instanceName, engine_instance_id, status, connected, accepted, action: "logout", loggedOut }` | current tenant only |
| `DELETE` | `/instance/id/:instanceID/logout` | owner, admin | none | same as above | current tenant only |

Runtime admin notes:

- `reconnect`, `pair`, and `logout` are mounted only on tenant-scoped SaaS routes; the backend does not expose the unsafe legacy global handlers.
- `pair` accepts either `phone` or `number` and returns the pairing code in both `code` and `pairingCode`.
- `logout` is intentionally honest about bridge limits: it requires an active logged-in runtime session and returns an error instead of pretending to clear a session that the bridge cannot prove exists.
- these action responses reuse the same top-level-plus-`data` compatibility envelope as `status`/`qr`, so the frontend can refresh operational state from the action response without an immediate second request.

### Status and QR

These endpoints intentionally return a compatibility envelope:

```json
{
  "message": "success",
  "data": { ...payload... },
  "...payload fields duplicated at top level..."
}
```

This keeps both `response.data` and `response.data.data` consumers working.

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/instance/:id/status` | owner, admin, agent | none | `{ instance_id, instanceName, engine_instance_id, status, connected }` envelope | current tenant only |
| `GET` | `/instance/id/:instanceID/status` | owner, admin, agent | none | same | current tenant only |
| `GET` | `/instance/:id/runtime` | owner, admin, agent | none | durable runtime status envelope with `durable`, optional `live`, and observability metadata | current tenant only |
| `GET` | `/instance/id/:instanceID/runtime` | owner, admin, agent | none | same | current tenant only |
| `GET` | `/instance/:id/runtime/history` | owner, admin, agent | query `limit?` | durable runtime event history with normalized lifecycle events | current tenant only |
| `GET` | `/instance/id/:instanceID/runtime/history` | owner, admin, agent | query `limit?` | same | current tenant only |
| `GET` | `/instance/:id/qr` | owner, admin, agent | none | `{ instance_id, instanceName, engine_instance_id, status, connected, qrcode, code }` envelope | current tenant only |
| `GET` | `/instance/:id/qrcode` | owner, admin, agent | none | same | current tenant only |
| `GET` | `/instance/id/:instanceID/qr` | owner, admin, agent | none | same | current tenant only |
| `GET` | `/instance/id/:instanceID/qrcode` | owner, admin, agent | none | same | current tenant only |

Fallback behavior:

- if QR is not available yet, status falls back to `connecting` or `open` depending on runtime state
- empty QR payloads are normalized to `qrcode: ""` and `code: ""`

Runtime observability notes:

- `/instance/*/runtime` is the tenant-safe status endpoint for lifecycle UX. It returns a durable status block sourced from the SaaS database and, when the bridge is reachable, an additional `live` block sourced from the current runtime snapshot.
- Durable lifecycle history currently records `connected`, `disconnected`, `pairing_started`, `paired`, `reconnect_requested`, `logout`, and `status_observed`.
- `status_observed` is a persisted "last seen" refresh event; it helps the frontend distinguish stale durable state from a recent live poll.
- `live` data remains bridge-dependent and may be missing when the legacy runtime is unavailable.
- `runtime/history` is durable per tenant and instance; it does not require the bridge for reads.

### Advanced settings

These routes bridge to the legacy instance model.

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/instance/:id/advanced-settings` | owner, admin, agent | none | `{ alwaysOnline, rejectCall, msgRejectCall, readMessages, ignoreGroups, ignoreStatus, instance_id, instanceName, engine_instance_id }` | current tenant only |
| `PUT` | `/instance/:id/advanced-settings` | owner, admin | `{ alwaysOnline, rejectCall, msgRejectCall, readMessages, ignoreGroups, ignoreStatus }` | `{ message, settings, instance_id, instanceName, engine_instance_id }` | current tenant only |
| `GET` | `/instance/id/:instanceID/advanced-settings` | owner, admin, agent | none | same as GET above | current tenant only |
| `PUT` | `/instance/id/:instanceID/advanced-settings` | owner, admin | same body | same response | current tenant only |

Notes:

- these settings are not persisted in the SaaS `instances` table today; they are stored in the bridged legacy instance model

### Instance-scoped integration routes

These routes were added from the frontend instance/integration gap report without reviving legacy `:instanceName` contracts. Every route below is tenant-scoped through auth + tenant middleware and resolves `:id` against the current tenant only.

#### Chat surface

| Method | Path | Roles | Request body | Success response | Status |
|---|---|---|---|---|---|
| `POST` | `/instance/:id/messages/text` | owner, admin, agent | `{ number, text, delay? }` | async queue response with `job_id`, `delivery_status`, and `status_endpoint` | implemented |
| `POST` | `/instance/id/:instanceID/messages/text` | owner, admin, agent | same as above | same | implemented |
| `GET` | `/instance/:id/messages/text/:jobID` | owner, admin, agent | none | text-job status with `status`, `delivery_status`, `sent`, `delivery_confirmed`, timestamps, `error?`, `message_id?` | implemented |
| `GET` | `/instance/id/:instanceID/messages/text/:jobID` | owner, admin, agent | none | same as above | implemented |
| `POST` | `/instance/:id/chats/search` | owner, admin, agent | `{ where? }` | `Chat[]` compatibility list sourced from live contacts/groups | implemented |
| `POST` | `/instance/:id/messages/search` | owner, admin, agent | legacy-compatible search payload with `where.key.remoteJid`, optional `limit`, `cursor`, `where.query`, `where.key.id` | `Message[]` chronological history response | implemented |
| `POST` | `/instance/:id/messages/media` | owner, admin, agent | media JSON payload, accepts either flat fields or nested `mediaMessage` | `{ message, instance_id, instanceName, engine_instance_id, data }` | implemented |
| `POST` | `/instance/:id/messages/audio` | owner, admin, agent | audio JSON payload, accepts either root `audio` or nested `audioMessage.audio` | `{ message, instance_id, instanceName, engine_instance_id, data }` | implemented |

Notes:

- `chats/search` is now functional, but it is a live runtime-backed list of contacts and groups, not a full persisted chat-history model.
- `messages/search` is now backed by a tenant-safe `ConversationMessage` read model in the SaaS database.
- Current persistence is strongest for outbound text/media/audio sends.
- Inbound messages are now persisted through both the active runtime bridge callback path and a tenant-safe inbound webhook fallback path when `DispatchInbound` includes enough message metadata.
- There is still no historical backfill or full upstream replay for sessions that were not observed by the current SaaS process.
- media/audio support is currently scoped to the JSON shapes used by the current frontend.

#### Event connectors and proxy

| Method | Path | Roles | Request body | Success response | Status |
|---|---|---|---|---|---|
| `GET` | `/instance/:id/websocket` | owner, admin, agent | none | `{ enabled, events }` | implemented |
| `PUT` | `/instance/:id/websocket` | owner, admin | `{ enabled, events }` or `{ websocket: { enabled, events } }` | `{ enabled, events }` | implemented |
| `GET` | `/instance/:id/rabbitmq` | owner, admin, agent | none | `{ enabled, events }` | implemented |
| `PUT` | `/instance/:id/rabbitmq` | owner, admin | `{ enabled, events }` or `{ rabbitmq: { enabled, events } }` | `{ enabled, events }` | implemented |
| `GET` | `/instance/:id/proxy` | owner, admin, agent | none | `{ enabled, host, port, protocol, username?, password? }` | implemented |
| `PUT` | `/instance/:id/proxy` | owner, admin | `{ enabled, host, port, protocol, username?, password? }` | same | implemented |
| `GET` | `/instance/:id/sqs` | owner, admin, agent | none | `501` partial response | partial |
| `PUT` | `/instance/:id/sqs` | owner, admin | any | `501` partial response | partial |

Implementation notes:

- websocket and rabbitmq settings bridge to the legacy instance model and sync runtime settings when the bridge is available
- proxy currently supports only `socks5`; any other protocol returns `400`
- when websocket/rabbitmq are enabled with no explicit events, the backend falls back to existing events or `["MESSAGE"]`

#### Unsupported integration suites registered as explicit partials

The following routes are intentionally registered and return `501` with a structured partial response because the current backend does not have tenant-safe repository/runtime support for them:

- `GET/PUT /instance/:id/chatwoot`
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

### Legacy compatibility routes still supported

These routes were added because the current sibling frontend still calls manager-style `:instanceName` paths for chat send/list operations. They remain tenant-scoped because auth uses bearer/API key plus tenant middleware, including legacy instance-token fallback.

| Method | Path | Roles | Request body | Success response | Status |
|---|---|---|---|---|---|
| `POST` | `/chat/findChats/:instanceName` | owner, admin, agent | `{ where? }` | same runtime-backed `Chat[]` list as SaaS chat search | implemented |
| `POST` | `/chat/findMessages/:instanceName` | owner, admin, agent | legacy-compatible message search payload | same `Message[]` history response as SaaS route | implemented |
| `POST` | `/message/sendText/:instanceName` | owner, admin, agent | `{ number, text, options? }` | legacy-style `{ message, data }` success response | implemented |
| `POST` | `/message/sendMedia/:instanceName` | owner, admin, agent | legacy-compatible media JSON payload | legacy-style `{ message, data }` success response | implemented |
| `POST` | `/message/sendWhatsAppAudio/:instanceName` | owner, admin, agent | legacy-compatible audio JSON payload | legacy-style `{ message, data }` success response | implemented |

Partial response shape:

```json
{
  "feature": "openai",
  "status": "partial",
  "implemented": false,
  "message": "This route is intentionally registered as a partial implementation because the current backend cannot complete it safely without reviving unsupported legacy patterns.",
  "instance_id": "uuid",
  "instanceName": "MyInstance",
  "blocked_by": ["..."]
}
```

## CRM

### Contacts

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/contacts` | owner, admin, agent | none | `[]Contact` with preloaded tags and notes | current tenant only |
| `POST` | `/contacts` | owner, admin, agent | `{ name, phone, email?, instance_id?, tags?, notes? }` | created `Contact` with tags/notes | current tenant only |
| `GET` | `/contacts/:id` | owner, admin, agent | none | `Contact` | current tenant only |
| `PATCH` | `/contacts/:id` | owner, admin, agent | `{ name?, phone?, email?, instance_id?, tags? }` | updated `Contact` | current tenant only |
| `POST` | `/contacts/:id/notes` | owner, admin, agent | `{ body }` | created `Note` | current tenant only |
| `POST` | `/contacts/:id/tags` | owner, admin, agent | `{ tags: []string }` | updated `Contact` | current tenant only |

Validation notes:

- contact create requires `name` and `phone`
- phone numbers are normalized to digits only
- duplicate phone per tenant returns `409`

## Broadcasts

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/broadcast` | owner, admin, agent | query `limit?` | `[]BroadcastJob` | tenant-only listing |
| `POST` | `/broadcast` | owner, admin, agent | `{ instance_id, message, rate_per_hour?, delay_sec?, max_attempts?, scheduled_at? }` | created `BroadcastJob` | target instance must belong to current tenant |
| `GET` | `/broadcast/:id` | owner, admin, agent | none | `BroadcastJob` | current tenant only |

Rate limiting:

- `POST /broadcast` is wrapped by the broadcast limiter

Current limitation:

- the processor is a stub and does not yet send WhatsApp messages itself

## Webhooks

### Tenant-managed endpoints

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `GET` | `/webhook` | owner, admin, agent | none | `[]WebhookEndpoint` | current tenant only |
| `POST` | `/webhook` | owner, admin | `{ name, url, inbound_enabled?, outbound_enabled?, signing_secret? }` | created `WebhookEndpoint` | current tenant only |
| `GET` | `/webhook/:id` | owner, admin, agent | none | `WebhookEndpoint` | current tenant only |

If both direction flags are false on create, the service defaults both to `true`.

### Legacy instance webhook compatibility

`GET /webhook?instanceName=<name>` and `POST /webhook` also support older frontend payloads for instance-level webhook sync.

Accepted legacy payload forms include:

- `instanceName`, `instance`, or `instance_id`
- `webhook_url`, `webhookUrl`, `url`
- nested `webhook: { enabled, url }`
- `events`, normalized to legacy values like `MESSAGE`

### Dispatch

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `POST` | `/webhook/inbound` | owner, admin, agent | `{ event_type, instance_id?, message_id?, data }` | `{ direction, results }` | current tenant only |
| `POST` | `/webhook/outbound` | owner, admin, agent | same | `{ direction, results }` | current tenant only |

Dispatch result item:

```json
{
  "endpoint_id": "uuid",
  "endpoint_name": "n8n",
  "url": "https://...",
  "delivered": true,
  "status_code": 200,
  "error": ""
}
```

Headers sent to target endpoints:

- `Content-Type: application/json`
- `X-Evolution-Tenant-ID`
- `X-Evolution-Event-Type`
- `X-Evolution-Direction`
- `X-Evolution-Signature` when a signing secret exists

Rate limiting:

- both dispatch endpoints are wrapped by the webhook limiter

AI side effect:

- inbound dispatch can enqueue AI auto-reply work when tenant + instance AI toggles are enabled

## Notes on Stale or Legacy API References

These routes are **not** registered in `cmd/api` and should be treated as stale frontend/legacy references:

- `/n8n/find/:instanceName`
- `/n8n/fetchSettings/:instanceName`
- `/websocket/find/:instanceName`
- `/websocket/set/:instanceName`
- `/rabbitmq/find/:instanceName`
- `/rabbitmq/set/:instanceName`
- `/sqs/find/:instanceName`
- `/sqs/set/:instanceName`
- `/proxy/find/:instanceName`
- `/proxy/set/:instanceName`
- `/chatwoot/find/:instanceName`
- `/chatwoot/set/:instanceName`
- older manager-oriented route sets documented in `docs/swagger.*`

Notes:

- The following compatibility chat routes are now registered and supported:
  - `/chat/findChats/:instanceName`
  - `/chat/findMessages/:instanceName`
  - `/message/sendText/:instanceName`
  - `/message/sendMedia/:instanceName`
  - `/message/sendWhatsAppAudio/:instanceName`
- For the SaaS layer, `internal/server/server.go` is the source of truth.
