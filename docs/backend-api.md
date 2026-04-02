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

### Connect / disconnect

| Method | Path | Roles | Request body | Success response | Tenant scope |
|---|---|---|---|---|---|
| `POST` | `/instance/:id/connect` | owner, admin | none | `{ message, instance_id, instanceName, status, qrcode?, code?, connected? }` | current tenant only |
| `POST` | `/instance/id/:instanceID/connect` | owner, admin | none | same as above | current tenant only |
| `POST` | `/instance/:id/disconnect` | owner, admin | none | `{ message, instance_id, instanceName, status }` | current tenant only |
| `POST` | `/instance/id/:instanceID/disconnect` | owner, admin | none | same as above | current tenant only |

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
| `GET` | `/instance/:id/qr` | owner, admin, agent | none | `{ instance_id, instanceName, engine_instance_id, status, connected, qrcode, code }` envelope | current tenant only |
| `GET` | `/instance/:id/qrcode` | owner, admin, agent | none | same | current tenant only |
| `GET` | `/instance/id/:instanceID/qr` | owner, admin, agent | none | same | current tenant only |
| `GET` | `/instance/id/:instanceID/qrcode` | owner, admin, agent | none | same | current tenant only |

Fallback behavior:

- if QR is not available yet, status falls back to `connecting` or `open` depending on runtime state
- empty QR payloads are normalized to `qrcode: ""` and `code: ""`

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
- older manager-oriented route sets documented in `docs/swagger.*`

For the SaaS layer, `internal/server/server.go` is the source of truth.
