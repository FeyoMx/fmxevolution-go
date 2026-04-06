# Backend Parity Report

Audited on 2026-04-05.

This report compares the current SaaS backend in `cmd/api` and `internal/*` against:

- the bundled upstream-style legacy surface still present in `pkg/routes/routes.go`, `pkg/sendMessage/*`, `pkg/user/*`, `pkg/group/*`, and `docs/wiki/*`
- the current sibling frontend repo under `../fmx-frontend/fmx-frontend`

The goal is practical parity, not route-count vanity. A route is only treated as matched when the current backend can support it truthfully with tenant scoping, auth, and runtime wiring.

## Executive Summary

The backend now has practical parity for:

- tenant auth and compatibility auth
- instance lifecycle basics
- QR and status
- advanced settings
- text sending
- media sending
- audio sending
- live chat listing
- tenant-safe message-history search
- webhook management
- websocket and rabbitmq instance config

The biggest remaining parity gaps are:

- durable inbound/backfill message-history parity
- Chatwoot and manager-style bot/integration suites
- SQS/Kafka parity
- richer runtime admin surfaces like pair/reconnect/logout parity
- real analytics parity

## Area-by-Area Status

| Area | Upstream / manager expectation | Current backend status | Frontend dependency | Parity status |
|---|---|---|---|---|
| Instance lifecycle | create, connect, disconnect, status, QR, advanced settings, admin lifecycle helpers | create/connect/disconnect/status/QR/advanced settings are mounted and functional; pair/reconnect/logout parity is still missing on SaaS routes | instance dashboard, settings, onboarding flows | partial |
| QR / pairing / status | QR and pair flows with compatibility payloads | QR + status are functional through legacy bridge; explicit pair route parity still missing | `manageInstance.tsx`, dashboard polling | partial |
| Text messaging | manager chat send and instance send | SaaS async send plus legacy compatibility send are functional | instance dashboard and chat UIs | matched for practical send |
| Media messaging | manager send media and instance send media | implemented via the legacy runtime bridge and SaaS/legacy compatibility routes | embed chat / manager chat composer | partial match |
| Audio messaging | WhatsApp audio / PTT send | implemented via the legacy runtime bridge and SaaS/legacy compatibility routes | embed chat / manager chat composer | partial match |
| Chat list | manager chat list | live runtime-backed contacts/groups list implemented | embed chat / manager chat list | partial match |
| Message history / search | manager chat message history | tenant-safe `ConversationMessage` read model now serves both SaaS and compatibility search routes; outbound persistence is solid, inbound persistence is partial and bridge-dependent | chat conversation pages | partial match |
| Webhook | endpoint registry + dispatch | implemented | webhook pages and n8n-style inbound/outbound flows | matched |
| WebSocket | per-instance config | implemented | websocket manager page | matched |
| RabbitMQ | per-instance config | implemented | rabbitmq manager page | matched |
| SQS | per-instance config | explicit `501 partial` | sqs manager page | unsupported |
| Kafka | connector parity | no SaaS route | no current SaaS page, upstream feature family exists | unsupported |
| Chatwoot | manager CRUD/settings | explicit `501 partial` | chatwoot page | unsupported |
| OpenAI | manager CRUD/settings/sessions/status | explicit `501 partial`; only tenant AI settings exist separately | openai pages | unsupported |
| Typebot | manager CRUD/settings/sessions/status | explicit `501 partial` | typebot pages | unsupported |
| Dify | manager CRUD/settings/sessions/status | explicit `501 partial` | dify pages | unsupported |
| N8N | manager CRUD/settings/sessions/status | explicit `501 partial`; webhook dispatch exists separately | n8n pages | unsupported |
| EvoAI | manager CRUD/settings/sessions/status | explicit `501 partial` | evoai pages | unsupported |
| EvolutionBot | manager CRUD/settings/sessions/status | explicit `501 partial` | evolutionBot pages | unsupported |
| Flowise | manager CRUD/settings/sessions/status | explicit `501 partial` | flowise pages | unsupported |
| Dashboard metrics | manager/business analytics | instance counts real, broader counters placeholder | dashboard pages | partial |
| Auth models and compatibility | bearer auth, API key, manager instance token compatibility | JWT + tenant API key + legacy instance-token mapping implemented | all protected pages | matched |

## Fully Matched Features

These are the areas where the current backend surface is functionally aligned enough for current product use.

### Auth models and tenant safety

- `Authorization: Bearer <token>`
- `X-API-Key` / `apikey`
- legacy instance-token fallback auth mapped back to tenant identity
- tenant middleware enforces current tenant scope

### Instance basics

- `POST /instance`
- `GET /instance`
- `GET /instance/:id`
- `GET /instance/id/:instanceID`
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
- `GET/PUT /instance/:id/advanced-settings`
- `GET/PUT /instance/id/:instanceID/advanced-settings`

### Messaging currently usable in product flows

- `POST /instance/:id/messages/text`
- `GET /instance/:id/messages/text/:jobID`
- `POST /instance/id/:instanceID/messages/text`
- `GET /instance/id/:instanceID/messages/text/:jobID`
- `POST /message/sendText/:instanceName`
- `POST /instance/:id/messages/media`
- `POST /instance/:id/messages/audio`
- `POST /message/sendMedia/:instanceName`
- `POST /message/sendWhatsAppAudio/:instanceName`

### Connector config already working

- `GET/PUT /instance/:id/websocket`
- `GET/PUT /instance/:id/rabbitmq`
- `GET/PUT /instance/:id/proxy`

### Webhook management

- `GET /webhook`
- `POST /webhook`
- `GET /webhook/:id`
- `POST /webhook/inbound`
- `POST /webhook/outbound`

## Partially Matched Features

### QR / pairing / status

What works:

- QR and status are implemented with compatibility envelopes.

What is still missing:

- explicit SaaS pair route parity
- reconnect/logout parity routes at SaaS level

Frontend dependency:

- instance dashboard and onboarding flows under the sibling frontend’s instance pages

### Media and audio

What works:

- current frontend JSON payloads using base64 or URL
- tenant scoping and legacy-runtime delivery

What is still weaker than upstream:

- not every upstream transport shape is exposed
- no dedicated async job tracking equivalent for media/audio yet

Frontend dependency:

- `src/lib/queries/chat/sendMessage.ts`
- embed chat and manager chat composer components

### Chat list

What works:

- contact and group list from live runtime state
- both SaaS and legacy compatibility routes

What is still missing:

- durable chat table
- labels parity
- reliable last-message ordering and preview parity

Frontend dependency:

- `src/lib/queries/chat/findChats.ts`
- `src/pages/instance/EmbedChatMessage/*`

### Dashboard metrics

What works:

- instance counts

What is still missing:

- reliable message/contact/revenue-style counters expected from a richer manager dashboard

## Unsupported Features

These remain unsupported today and should not be represented as complete product features.

### Message history / message search

Implemented routes:

- `POST /instance/:id/messages/search`
- `POST /chat/findMessages/:instanceName`

Current limitations:

- outbound text/media/audio history is persisted, but inbound capture still depends on active runtime events reaching the current SaaS process
- there is no backfill from older sessions or full upstream history sync into the SaaS read model
- chat history remains queryable by tenant, instance, and `remoteJid`, but it is not yet a complete replacement for every upstream replay path

Frontend pages now unblocked:

- `src/lib/queries/chat/findMessages.ts`
- `src/pages/instance/Chat/messages.tsx`
- `src/pages/instance/EmbedChatMessage/Messages/*`

### Manager integration suites

Unsupported routes still mounted as explicit partials:

- `GET/PUT /instance/:id/chatwoot`
- `GET/PUT /instance/:id/sqs`
- all mounted `openai`, `typebot`, `dify`, `n8n`, `evoai`, `evolutionBot`, and `flowise` instance CRUD/settings/session/status routes

Why unsupported:

- no tenant-safe repository model
- no SaaS-owned runtime/session model for those managers
- reviving raw legacy manager handlers would violate tenant safety and architecture goals

Frontend pages blocked:

- `/manager/instance/:instanceId/chatwoot`
- `/manager/instance/:instanceId/sqs`
- `/manager/instance/:instanceId/openai`
- `/manager/instance/:instanceId/typebot`
- `/manager/instance/:instanceId/dify`
- `/manager/instance/:instanceId/n8n`
- `/manager/instance/:instanceId/evoai`
- `/manager/instance/:instanceId/evolutionBot`
- `/manager/instance/:instanceId/flowise`

### Still-missing upstream behavior

- Kafka connector parity
- full pairing/reconnect/logout admin parity on SaaS routes
- full broadcast-to-WhatsApp delivery parity
- trustworthy analytics parity

## Legacy Routes Currently Returning `501`

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

## Recommended Implementation Order

1. Durable inbound history parity: backfill or replay inbound history beyond what the active bridge observes at runtime.
2. Runtime admin parity: add reconnect/logout/pair route parity without bypassing tenant/auth architecture.
3. Integration parity: implement only the manager integration suites that are real product priorities, starting with the pages most actively used by the frontend.
4. Metrics parity: replace placeholder dashboard counters with real aggregates.
5. Cleanup: reduce remaining bridge-only logic and stale Swagger/doc artifacts.
