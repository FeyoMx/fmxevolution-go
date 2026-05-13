# Pilot Operations Runbook

Last updated: 2026-05-07.

This runbook is for backend pilot support of the SaaS API served by `cmd/api`. It assumes the API runs behind a reverse proxy and writes structured JSON logs to stdout or to the local `api*.log` files used by the current VPS workflow.

Use production credentials carefully. Do not paste bearer tokens, tenant API keys, JWT secrets, database passwords, webhook signing secrets, or full customer message bodies into tickets.

## Required Context

Collect these values before troubleshooting:

- API base URL, for example `https://api.example.com`
- tenant ID or tenant slug
- instance ID from the SaaS `instances.id` column or API response
- broadcast ID when debugging a campaign
- `X-Request-ID` response header from a failed API call
- UTC timestamp window for the report

## Check Service Health

Public checks:

```bash
curl -fsS https://api.example.com/livez
curl -fsS https://api.example.com/healthz
curl -fsS https://api.example.com/readyz
```

Expected responses:

- `/livez`: `{"status":"alive"}` when the process is serving HTTP.
- `/healthz`: `{"status":"ok"}` when the API route is reachable.
- `/readyz`: `{"status":"ready"}` when PostgreSQL can be pinged within the readiness timeout.

If `/livez` is up but `/readyz` returns `503`, the process is alive and the first place to check is database reachability, credentials, firewall rules, and PostgreSQL connection limits.

Local VPS check:

```powershell
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/readyz
```

## Tail Logs

For systemd deployments:

```bash
journalctl -u fmx-api -f -o cat
journalctl -u fmx-api --since "30 minutes ago" -o cat
```

For Docker deployments:

```bash
docker logs -f fmx-api
docker logs --since 30m fmx-api
```

For the current local/VPS file workflow:

```powershell
Get-Content .\api.runtime.out.log -Wait -Tail 200
Get-Content .\api.runtime.err.log -Wait -Tail 200
Get-Content .\api.out.log -Wait -Tail 200
Get-Content .\api.err.log -Wait -Tail 200
```

Useful log fields:

- `request_id`: correlate an HTTP response with backend logs.
- `tenant_id`: isolate one pilot tenant.
- `instance_id`: isolate one WhatsApp runtime instance.
- `broadcast_id`: isolate one broadcast workflow. Older log lines may also use `job_id`; they refer to the same broadcast job ID.
- `module`: identify the subsystem, such as `instance`, `broadcast`, `webhook`, or `ai`.

Keep `LOG_LEVEL=info` for normal pilot operation. Temporarily use `LOG_LEVEL=debug` only during active troubleshooting because recipient-level broadcast attempts are debug logs.

## Inspect Runtime Status

Use the authenticated tenant API. Replace placeholders with a valid bearer token and instance ID:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/instance/$INSTANCE_ID/runtime
```

Compatibility alias:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/instance/id/$INSTANCE_ID/runtime
```

Read the durable fields first: status, `connected`, `logged_in`, `pairing_active`, `last_event_type`, `last_error`, and the last event timestamps. The optional live snapshot still depends on the legacy WhatsApp bridge, so a live failure does not always mean the durable state is missing.

## Inspect Runtime History

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/instance/$INSTANCE_ID/runtime/history?limit=50"
```

Use history to distinguish:

- connection lifecycle changes: `connected`, `disconnected`, `reconnect_requested`, `logout`
- pairing activity: `pairing_started`, `paired`
- status probes: `status_observed`
- replay/backfill activity: `history_sync_requested`, `history_sync`

Correlate timestamps with logs using `tenant_id`, `instance_id`, and any `request_id` returned by the API call that triggered the event.

## Check Broadcast Recipient Progress

Summary and job state:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/broadcast/$BROADCAST_ID
```

Recipient page:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/broadcast/$BROADCAST_ID/recipients?page=1&limit=100"
```

Failed recipients only:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/broadcast/$BROADCAST_ID/recipients?status=failed&page=1&limit=100"
```

Search by phone or contact fragment:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/broadcast/$BROADCAST_ID/recipients?query=521555&page=1&limit=50"
```

Statuses:

- `pending`: no confirmed send evidence yet, or waiting for retry.
- `sent`: the send path returned a message ID, server ID, timestamp, or chat JID.
- `delivered`: a runtime receipt matched the recipient message.
- `read`: a runtime read receipt matched the recipient message.
- `failed`: permanent recipient-level failure.

## Diagnose Failed Sends

1. Capture `request_id`, `tenant_id`, `instance_id`, `broadcast_id`, recipient phone, and timestamp.
2. Check `/readyz` to rule out database outage.
3. Check runtime status for the instance. If `connected=false` or `logged_in=false`, resolve the WhatsApp session before retrying the broadcast.
4. Check runtime history for disconnect, logout, pairing, or bridge-unavailable events around the failure time.
5. Check broadcast recipient progress for `last_error`, `attempt_count`, `last_attempt_at`, and `failed_at`.
6. Search logs by `request_id` for API calls, by `broadcast_id` for campaign work, and by `instance_id` for runtime bridge failures.
7. Treat missing delivered/read receipts as best-effort runtime visibility, not necessarily message failure. `sent` is the durable evidence that the send call succeeded.

Common signals:

- `bridge unavailable`, `runtime unavailable`, or `connect legacy runtime failed`: live WhatsApp bridge issue.
- `recipient verification failed; using parsed jid`: number verification fell back to parsed JID and may need contact validation.
- `broadcast recipient send returned no delivery evidence`: send path did not produce durable evidence and the job will retry if attempts remain.
- `/readyz` `503` plus repository errors: database connectivity or pool saturation.

## Verify Existing Webhooks

Use these read-only checks when a webhook already exists and the goal is inspection. Do not call `POST /webhook` for verification because that route is the create/update path.

List tenant-managed webhook endpoints:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/webhook
```

Inspect one tenant-managed endpoint by ID:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/webhook/$WEBHOOK_ENDPOINT_ID
```

Inspect legacy instance webhook preferences:

```bash
curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/webhook?instanceName=$INSTANCE_NAME"
```

Expected instance preference fields:

- `enabled`
- `instanceName`
- `url`, `webhook`, and `webhook_url`
- `events`
- `webhookBase64`
- `webhookByEvents`

Safe delivery test:

1. Prefer a real tenant event, such as sending one opted-in inbound test message to the configured WhatsApp instance.
2. Confirm the external webhook receiver records the event.
3. Check API logs for `webhook delivered` or `webhook delivery failed`.
4. If a manual dispatch is necessary, use `POST /webhook/inbound` or `POST /webhook/outbound` only with an explicit test payload and only after confirming the downstream automation can safely receive it. These routes do not create endpoints, but they do call existing external webhook URLs.

Avoid duplicate webhook creation:

- Do not run `POST /webhook` just to check whether a webhook exists.
- Do not copy a working endpoint into a second endpoint record unless duplicate downstream delivery is intentional.
- If the goal is instance-level verification, use `GET /webhook?instanceName=...`.
- If the goal is tenant endpoint verification, use `GET /webhook` and then `GET /webhook/:id`.

Useful log filters:

```bash
grep -E 'webhook delivered|webhook delivery failed|legacy webhook updated' api*.log
grep -E 'endpoint_id|tenant_id|direction|event_type|status_code' api*.log | tail -n 100
```

## Restart Backend Safely

Before restart:

```bash
curl -fsS https://api.example.com/readyz
```

Graceful systemd restart:

```bash
sudo systemctl restart fmx-api
sudo systemctl status fmx-api --no-pager
journalctl -u fmx-api --since "5 minutes ago" -o cat
```

Graceful Docker restart:

```bash
docker restart --time 30 fmx-api
docker logs --since 5m fmx-api
```

Manual VPS process workflow:

```powershell
Get-Content .\api.pid
Stop-Process -Id (Get-Content .\api.pid) -ErrorAction Stop
Start-Process -FilePath .\api.exe -RedirectStandardOutput .\api.runtime.out.log -RedirectStandardError .\api.runtime.err.log -WindowStyle Hidden
```

The API handles `SIGINT` and `SIGTERM`, stops accepting HTTP requests, cancels background workers, waits up to `HTTP_SHUTDOWN_TIMEOUT`, and closes database connections. Avoid forced kills unless the process does not exit after the configured shutdown timeout.

After restart:

```bash
curl -fsS https://api.example.com/livez
curl -fsS https://api.example.com/readyz
```

Then inspect logs for `api server starting`, `shutdown signal received`, `stop background workers`, and database or migration errors.

## Verify Database Connectivity

API-level check:

```bash
curl -i https://api.example.com/readyz
```

Direct PostgreSQL check:

```bash
psql "$DATABASE_URL" -c "select now() as db_time, current_database() as database, current_user as user;"
```

Connection pressure:

```sql
select state, count(*)
from pg_stat_activity
where datname = current_database()
group by state
order by count(*) desc;
```

If direct `psql` succeeds but `/readyz` fails, verify that the running process has the same `DATABASE_URL` and that PostgreSQL allows connections from the API host.

## Operational SQL Snippets

Set a tenant filter when possible:

```sql
-- Replace with the pilot tenant UUID.
\set tenant_id '00000000-0000-0000-0000-000000000000'
```

List tenants:

```sql
select id, slug, name, ai_enabled, created_at, updated_at
from tenants
order by created_at desc;
```

List users:

```sql
select u.id, u.tenant_id, t.slug as tenant_slug, u.email, u.name, u.role, u.created_at, u.updated_at
from users u
join tenants t on t.id = u.tenant_id
where u.tenant_id = :'tenant_id'
order by u.created_at desc;
```

List instances:

```sql
select i.id, i.tenant_id, t.slug as tenant_slug, i.name, i.status, i.engine_instance_id,
       i.ai_enabled, i.ai_auto_reply, i.created_at, i.updated_at
from instances i
join tenants t on t.id = i.tenant_id
where i.tenant_id = :'tenant_id'
order by i.updated_at desc;
```

Runtime status by instance:

```sql
select s.tenant_id, s.instance_id, i.name, s.status, s.last_seen_status,
       s.last_event_type, s.connected, s.logged_in, s.pairing_active,
       s.disconnect_reason, s.last_error, s.last_event_at, s.last_seen_at, s.updated_at
from runtime_session_states s
join instances i on i.id = s.instance_id
where s.tenant_id = :'tenant_id'
order by s.updated_at desc;
```

Recent runtime events:

```sql
select e.occurred_at, e.tenant_id, e.instance_id, i.name, e.event_type, e.event_source,
       e.status, e.connected, e.logged_in, e.pairing_active,
       e.disconnect_reason, e.error_message, e.message
from runtime_session_events e
join instances i on i.id = e.instance_id
where e.tenant_id = :'tenant_id'
order by e.occurred_at desc
limit 100;
```

Recent broadcast jobs:

```sql
select b.id as broadcast_id, b.tenant_id, t.slug as tenant_slug, b.instance_id, i.name as instance_name,
       b.status, b.attempts, b.max_attempts, b.worker_id, b.last_error,
       b.available_at, b.started_at, b.completed_at, b.failed_at, b.created_at, b.updated_at
from broadcast_jobs b
join tenants t on t.id = b.tenant_id
join instances i on i.id = b.instance_id
where b.tenant_id = :'tenant_id'
order by b.created_at desc
limit 50;
```

Broadcast recipient summary:

```sql
select broadcast_id,
       count(*) as total,
       count(*) filter (where attempt_count > 0) as attempted,
       count(*) filter (where delivery_status in ('sent', 'delivered', 'read')) as sent_or_better,
       count(*) filter (where delivery_status = 'delivered') as delivered,
       count(*) filter (where delivery_status = 'read') as read,
       count(*) filter (where delivery_status = 'failed') as failed,
       count(*) filter (where delivery_status = 'pending') as pending
from broadcast_recipient_progress
where tenant_id = :'tenant_id'
group by broadcast_id
order by max(updated_at) desc
limit 50;
```

Failed broadcast recipients:

```sql
select p.broadcast_id, p.instance_id, i.name as instance_name, p.phone,
       p.delivery_status, p.attempt_count, p.last_error,
       p.last_attempt_at, p.failed_at, p.message_id, p.chat_jid, p.updated_at
from broadcast_recipient_progress p
join instances i on i.id = p.instance_id
where p.tenant_id = :'tenant_id'
  and p.delivery_status = 'failed'
order by coalesce(p.failed_at, p.updated_at) desc
limit 100;
```

Recipient lookup by phone:

```sql
select p.broadcast_id, p.phone, p.delivery_status, p.attempt_count, p.last_error,
       p.sent_at, p.delivered_at, p.read_at, p.failed_at, p.message_id, p.chat_jid
from broadcast_recipient_progress p
where p.tenant_id = :'tenant_id'
  and p.phone like '%521555%'
order by p.updated_at desc
limit 50;
```

Recent webhook failures:

```sql
select d.created_at, d.tenant_id, e.name as endpoint_name, d.direction, d.event_type,
       d.status, d.response_status, d.error_message
from webhook_deliveries d
join webhook_endpoints e on e.id = d.endpoint_id
where d.tenant_id = :'tenant_id'
  and d.status <> 'delivered'
order by d.created_at desc
limit 50;
```

## Pilot Support Risks

- Runtime live snapshots and WhatsApp lifecycle actions still depend on the legacy bridge; durable state can be available while live actions fail.
- Delivered/read broadcast progression is best-effort and requires receipt events that can be matched back to recipient progress.
- Historical broadcast jobs created before recipient progress tracking may have partial recipient analytics.
- Chat history remains SaaS-observed and bridge-dependent; it is not a universal full WhatsApp archive.
- Rate limiting is memory-backed by default, so limits reset on restart and are not shared across multiple API replicas.
