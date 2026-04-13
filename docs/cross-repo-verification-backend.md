# Cross-Repo Verification: Backend-Led Frontend Alignment

Date: 2026-04-12

Backend source-of-truth reviewed:
- `README.md`
- `CHANGELOG.md`
- `docs/backend-product-readiness.md`
- `docs/backend-api.md`
- `internal/server/server.go`

Frontend consumer surface reviewed:
- `../fmx-frontend/fmx-frontend/src/routes/index.tsx`
- `../fmx-frontend/fmx-frontend/src/components/sidebar.tsx`
- `../fmx-frontend/fmx-frontend/src/lib/queries/api.ts`
- `../fmx-frontend/fmx-frontend/src/lib/queries/chat/tenantChat.ts`
- `../fmx-frontend/fmx-frontend/src/lib/queries/instance/manageInstance.tsx`
- `../fmx-frontend/fmx-frontend/src/lib/queries/instance/runtime.ts`
- `../fmx-frontend/fmx-frontend/src/lib/queries/webhook/fetchWebhook.ts`
- `../fmx-frontend/fmx-frontend/src/lib/queries/webhook/manageWebhook.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/Dashboard/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/CRM/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/Broadcast/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/AISettings/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/APIKeys/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/instance/DashboardInstance/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/instance/Webhook/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/instance/Websocket/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/instance/Rabbitmq/index.tsx`
- `../fmx-frontend/fmx-frontend/src/pages/instance/Proxy/index.tsx`

## Frontend surfaces fully supported by backend

- Tenant-safe auth/session bootstrapping is aligned. The frontend uses `/auth/me` and `/tenant`, and the backend mounts both behind authenticated SaaS middleware.
- The main manager navigation is aligned with currently supported backend surface:
  - dashboard
  - contacts
  - chats
  - broadcast
  - AI settings
  - API keys
- Tenant-safe chat pages are aligned with current backend SaaS routes:
  - `POST /instance/:id/chats/search`
  - `POST /instance/:id/messages/search`
  - `POST /instance/:id/messages/text`
  - `POST /instance/:id/messages/media`
  - `POST /instance/:id/messages/audio`
- Contacts UI is aligned with current backend CRM support. The frontend stays within the exposed backend contact surface and already avoids claiming delete/pipeline automation that the backend does not expose.
- Broadcast UI is aligned at the route level with:
  - `GET /broadcast`
  - `POST /broadcast`
  - `GET /broadcast/:id`
- AI settings UI is aligned with the backend MVP surface:
  - `GET /ai/settings`
  - `PUT /ai/settings`
  - `GET /ai/instances/:instanceID`
  - `PUT /ai/instances/:instanceID`
- Instance configuration pages for webhook, websocket, rabbitmq, and proxy are aligned with currently mounted backend routes and already tolerate partial support honestly.
- API keys UI is aligned as an informational page only. It does not claim backend self-service key management that is not currently mounted.

## Frontend surfaces partially supported by backend

- Dashboard metrics are only partially backed by real backend aggregates. The backend exposes `/dashboard/metrics`, but instance totals are the only clearly trustworthy counters today. Other totals remain limited or placeholder-level.
- Runtime observability is partially supported. The backend supports runtime snapshot and durable history routes, but live health and event completeness still depend on the runtime bridge and on what the SaaS process has actually observed.
- Broadcast is now real send-attempt capable in the backend, but operator-facing analytics remain limited. The frontend is correct to stay conservative on totals and per-recipient insight.
- Chat send actions are supported, but real delivery still depends on the selected instance runtime being available. The frontend capability model is slightly more optimistic than the backend runtime reality.
- Webhook, websocket, rabbitmq, and proxy pages are mounted against real backend routes, but these areas still include partial or environment-dependent behavior and should continue to present graceful not-implemented states when returned by the API.

## Frontend surfaces that are misleading relative to backend reality

- The instance dashboard lifecycle actions are not aligned with current backend route truth:
  - frontend `restart` uses `POST /instance/id/:instanceId/restart`
  - backend mounts `POST /instance/id/:instanceID/reconnect`
  - frontend `logout` uses `POST /instance/id/:instanceId/logout`
  - backend mounts `DELETE /instance/id/:instanceID/logout`
- The instance dashboard pairing flow is also misaligned:
  - frontend calls `connect({ instanceId, number })`
  - backend separates generic connect from pairing and mounts `POST /instance/id/:instanceID/pair`
  - this means the visible pairing/connect UX is still relying on an outdated contract
- The broadcast page now slightly understates backend capability. Backend code has moved beyond queue-only behavior and now performs real WhatsApp text send attempts through the selected instance runtime. The frontend copy still reads closer to queue/review-only semantics.
- The frontend codebase still contains legacy query modules for unsupported integration suites and instance-token flows. Much of this residue is not on the main visible route path, but it remains misleading as implementation inventory because the backend still returns `501` partial for these suites.

## Routes/pages that should remain gated

- These frontend routes should remain gated because the backend still mounts them as partial or unsupported surfaces:
  - `/manager/instance/:instanceId/openai`
  - `/manager/instance/:instanceId/typebot`
  - `/manager/instance/:instanceId/dify`
  - `/manager/instance/:instanceId/n8n`
  - `/manager/instance/:instanceId/evoai`
  - `/manager/instance/:instanceId/evolutionBot`
  - `/manager/instance/:instanceId/flowise`
  - `/manager/instance/:instanceId/sqs`
  - `/manager/instance/:instanceId/chatwoot`
- Legacy embed chat surfaces should remain gated:
  - `/manager/embed-chat`
  - `/manager/embed-chat/:remoteJid`
- Any frontend path that implies full bot-suite management, SQS, Chatwoot, or bridge-independent orchestration should stay hidden or explicitly marked unavailable until the backend supports it without `501 partial`.

## Any response-shape mismatch risk

- The largest current risk is not JSON envelope shape but route/method mismatch on visible instance lifecycle actions.
- The frontend has defensive normalizers for instances, runtime snapshots, and chat payloads, which reduces risk from envelope variation.
- Webhook reads are reasonably resilient because the frontend accepts either `response.data` or `response.data.data`.
- Instance cards and summary widgets still infer counts from normalized instance payloads. Backend support for those counts is not a stable product contract, so the UI should continue to tolerate null or unknown values.
- Chat capability labeling in the frontend currently treats text/media/audio send as broadly available. Backend reality is narrower: those sends can fail honestly when the selected runtime is disconnected or unavailable.

## Any auth/tenant mismatch risk

- The active tenant-safe frontend surface mostly aligns with backend SaaS auth:
  - bearer token auth
  - tenant-scoped user identity from `/auth/me`
  - tenant metadata from `/tenant`
- The frontend still ships a legacy `api` client that relies on instance-token style headers and legacy manager-era query modules. This is a codebase hygiene risk more than a current visible UX risk, but it increases the chance of accidental reuse of non-MVP patterns.
- Cross-tenant leakage risk appears controlled on the active SaaS surface because the frontend passes tenant context and the backend enforces tenant/instance ownership on protected routes.
- The biggest auth/tenant risk is future accidental regression if frontend work reactivates old instance-token query modules instead of the current tenant-safe `apiGlobal` path.

## Any UX text that overstates backend capability

- Most current visible MVP text is appropriately conservative.
- The larger UX problem is not overstatement but stale action wiring on the instance dashboard, where visible controls imply lifecycle actions that do not match current backend route truth.
- Dashboard copy remains appropriately cautious about metrics completeness.
- AI settings copy remains appropriately cautious that legacy bot/integration CRUD is outside the MVP path.

## Top 3 frontend changes that would improve alignment

1. Fix instance dashboard lifecycle actions to match backend route truth:
   - replace `restart` usage with backend `reconnect`
   - switch logout to `DELETE /instance/id/:instanceID/logout`
   - use `POST /instance/id/:instanceID/pair` for pairing flows instead of overloading connect
2. Update broadcast UX copy to reflect current backend reality:
   - broadcast now performs real WhatsApp text send attempts
   - keep analytics language conservative because per-recipient telemetry is still limited
3. Remove or isolate legacy frontend query modules that still target unsupported bot/integration suites and instance-token-era paths so they cannot accidentally re-enter the visible operator surface

## Overall assessment

- The current frontend is mostly MVP-aligned with the backend on the visible SaaS operator path.
- The biggest remaining mismatch is the instance dashboard lifecycle/pairing contract, because it is a live operator surface and not just dormant legacy residue.
- The product can be considered broadly MVP-aligned for tenant-safe chat, runtime observability, contacts, broadcast, and AI settings, but not fully aligned until the instance dashboard action wiring is corrected.

## Recommended next coordinated task

- Make a frontend-only alignment pass on `DashboardInstance` and its instance management query layer so connect, pair, reconnect, and logout use the current backend SaaS routes and methods exactly.
