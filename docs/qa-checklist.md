# QA Checklist

Audited on 2026-04-06.

This checklist is intended for manual MVP release candidate validation.

## Smoke check

- Verify the API boots successfully.
- Call `GET /healthz` and confirm `{ "status": "ok" }`.
- Confirm protected routes reject missing auth with a consistent error envelope.

## Optional QA seed data

The backend includes a development-only QA seed command for dense manual testing. It is disabled by default and refuses `APP_ENV=production` or `APP_ENV=prod`.

Example PowerShell usage against a development database:

```powershell
$env:APP_ENV = "development"
$env:QA_SEED_ENABLED = "true"
$env:DATABASE_URL = "postgres://user:pass@localhost:5432/fmx_dev?sslmode=disable"
$env:JWT_SECRET = "local-dev-secret"
go run ./cmd/qa-seed -tenant-slug qa-seed -create-tenant=true
```

Default created login, when the tenant/admin does not already exist:

- tenant slug: `qa-seed`
- admin email: `qa.admin@example.test`
- admin password: `QaSeed123!`

Fixture coverage:

- 125 tenant-scoped contacts
- one dense instance plus one intentionally sparse instance
- six broadcast jobs, including one historical job with no recipient snapshot
- 150 broadcast recipient progress rows across `pending`, `sent`, `delivered`, `read`, and `failed`
- 240 conversation messages across three chats
- durable runtime state plus mixed lifecycle/history events

The command is deterministic and safe to rerun for the same tenant; it updates the QA fixture rows instead of creating unbounded duplicates.

## Auth

- Log in with a valid tenant slug, email, and password.
- Refresh a valid refresh token.
- Call `GET /auth/me` and confirm tenant identity, role, `api_key`, and `api_key_auth` are present.
- Call `POST /auth/logout` and confirm the stateless acknowledgement response includes `accepted: true`.

## Tenant

- Create a tenant with valid bootstrap input.
- Confirm trimmed input is accepted and tenant slug uniqueness is enforced.
- Confirm short admin passwords are rejected.
- Call `GET /tenant` under authenticated context and confirm only the current tenant is returned.

## Instance lifecycle

- Create a tenant-scoped instance.
- Fetch the instance by both `:id` and `id/:instanceID` where applicable.
- Exercise `connect`, `disconnect`, `reconnect`, `pair`, and `logout`.
- Confirm runtime-action responses include clear operator-facing fields such as `operator_message`, `bridge_dependent`, and `status_refresh`.
- Confirm cross-tenant instance access is rejected.

## Runtime and history

- Call `GET /instance/:id/status`.
- Call `GET /instance/:id/runtime`.
- Call `GET /instance/:id/runtime/history`.
- Confirm durable runtime reads work even when live runtime details are sparse.
- Confirm runtime/history responses remain honest when the bridge is unavailable or incomplete.

## Chat list and detail

- Call the supported chat list/search route for an active instance.
- Confirm the response shape is usable for current frontend needs.
- Confirm the operator understands that the list is live-bridge-backed, not a full durable chat table.
- Confirm rate-limit or upstream failures surface as honest errors rather than fake empty success.

## Text, media, and audio send

- Send text through the SaaS send route.
- Poll text job status and confirm lifecycle states are reasonable.
- Send media with a currently supported payload shape.
- Send audio with a currently supported payload shape.
- Confirm outbound messages appear in the message-history read model when expected.

## Message history and backfill

- Query message history/search for a tenant-scoped instance and chat.
- Confirm inbound and outbound messages already observed by the system can be searched.
- Trigger `POST /instance/:id/history/backfill` with a valid anchor.
- Confirm accepted backfill responses are explicit about bridge dependence.
- Confirm malformed `timestamp` or `messageInfo.timestamp` values are rejected.

## Contacts

- Create a contact with valid name and phone.
- Update a contact.
- Add notes and tags.
- Confirm duplicate phone values are rejected per tenant.
- Confirm phone values that normalize to no digits are rejected.

## Broadcast basic validation

- Create a broadcast job with valid input.
- List broadcast jobs with and without the `limit` query parameter.
- Confirm negative `delay_sec`, `rate_per_hour`, or `max_attempts` values are rejected.
- Confirm `GET /broadcast/:id/recipients` defaults omitted `page`/`limit`, rejects negative pagination, and displays mixed recipient states from the QA seed.
- Confirm broadcast detail is tenant-scoped.
- Confirm operators understand that delivery execution is real but receipt progression beyond `sent` remains runtime-dependent.

## AI settings

- Get tenant AI settings.
- Update tenant AI settings with a supported provider and valid `model` and `base_url`.
- Get and update per-instance AI toggles.
- Confirm unsupported provider values are rejected.

## Negative and error cases

- Missing auth on protected routes.
- Tenant mismatch headers on protected routes.
- Cross-tenant instance access attempts.
- Invalid JSON payloads.
- Missing required fields for auth, tenant, contact, broadcast, and backfill routes.
- Runtime actions attempted with bridge/session conditions that cannot satisfy the request.
- Bridge unavailability during runtime or chat operations.
- Confirm supported routes return consistent `{ error, message, code }` error envelopes.
