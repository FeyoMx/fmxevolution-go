# RC1 Real-World Validation

This document is the RC1 validation runbook and findings log for testing the backend against a real WhatsApp instance. It is intentionally separate from synthetic QA seed validation because RC1 must exercise the legacy runtime bridge, actual pairing, real sends, and real receipt behavior.

Current status: prepared for execution, not yet completed in this workspace.

## Scope

Validate the deployable API under real WhatsApp usage conditions:

- Real instance creation and pairing
- Runtime reconnect and durable runtime history
- Text, media, and audio sending
- Best-effort message status progression
- Small-audience broadcast delivery
- Runtime stability, log volume, CPU, and memory behavior

## Required Setup

Environment:

- API built from `./cmd/api`
- PostgreSQL database reachable by `DATABASE_URL`
- `APP_ENV=production` or staging-equivalent
- `PORT` set to the API listen port
- `JWT_SECRET` set to at least 32 random characters
- `LOG_LEVEL=info` for normal validation, `LOG_LEVEL=debug` only for short troubleshooting windows
- Reverse proxy or direct local access to the API port

Accounts and data:

- One test tenant
- One owner/admin API user
- One real WhatsApp device/account dedicated to validation
- Two or three opted-in recipient phone numbers for send and broadcast checks
- One small media asset and one small audio asset suitable for a real WhatsApp send

Safety rules:

- Use only opted-in recipients.
- Keep broadcast audience to two or three contacts.
- Use low send volume and wait between operations.
- Do not test with production client data until this document records a ready verdict.

## RC1 Activation Mode

Run the first real-world session in a quiet, observable mode:

- Use `LOG_LEVEL=info`.
- Do not use `LOG_LEVEL=debug` during normal testing.
- Enable `LOG_LEVEL=debug` only for a short, targeted retry when a specific send or runtime state is ambiguous.
- Keep one terminal open for API logs.
- Keep one terminal open for health checks and API requests.
- Keep one terminal open for process/database observation.

Expected log shape:

- HTTP request logs include `request_id`, `tenant_id`, and route-derived `instance_id` when the route carries an instance reference.
- Pair/reconnect lifecycle logs include `tenant_id` and `instance_id`.
- Send failures include `tenant_id`, `instance_id`, and message/job context when available.
- Broadcast logs include job, tenant, instance, recipient totals, and failure/resume state.
- Logs should not include passwords, tokens, request bodies, media payloads, webhook signing secrets, or raw JWT/API keys.

No extra temporary runtime logs were added for RC1 activation. Existing info/warn/error logs are expected to be sufficient; debug-level recipient attempt logs should stay disabled unless actively troubleshooting.

## Startup And Health

Commands:

```bash
go build -mod=readonly -o api ./cmd/api

APP_ENV=production \
LOG_LEVEL=info \
DATABASE_URL='postgres://user:password@host:5432/fmx?sslmode=disable' \
JWT_SECRET='replace-with-at-least-32-random-characters' \
PORT=8080 \
./api
```

Expected:

- API starts without config errors.
- `GET /livez` returns `200`.
- `GET /healthz` returns `200`.
- `GET /readyz` returns `200` when PostgreSQL is reachable.
- Missing or invalid env fails fast before serving traffic.

Safe run command for RC1:

```bash
APP_ENV=production \
LOG_LEVEL=info \
DATABASE_URL='postgres://user:password@host:5432/fmx?sslmode=disable' \
JWT_SECRET='replace-with-at-least-32-random-characters' \
PORT=8080 \
./api 2>&1 | tee rc1-api.log
```

Observe from another terminal:

```bash
curl -fsS http://127.0.0.1:8080/livez
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
```

Basic process checks:

```bash
ps -o pid,pcpu,pmem,rss,etime,cmd -p "$(pgrep -f './api' | head -n 1)"
```

Basic PostgreSQL checks:

```sql
SELECT count(*) FROM pg_stat_activity WHERE datname = current_database();
```

Findings:

- Build validation in this workspace: passed for `go build -mod=readonly ./cmd/api`.
- Manual startup with intentionally invalid `PORT=abc`: failed fast with `PORT must be an integer between 1 and 65535`.
- Full real startup against PostgreSQL: not executed here because no deployment database was provided in this session.

## Auth And Tenant Smoke

Steps:

1. Create or confirm the test tenant.
2. Log in with `POST /auth/login`.
3. Store the bearer token.
4. Call `GET /auth/me`.
5. Confirm protected routes reject unauthenticated requests.

Expected:

- Login succeeds for valid credentials.
- Protected routes return `401` without auth.
- Tenant context is correct in `GET /auth/me`.
- Logs include `request_id` and `tenant_id` without tokens, passwords, or request bodies.

Findings:

- Not executed against a real deployment in this session.

## Real Instance Pairing

Steps:

1. Create an instance with `POST /instance`.
2. Request QR with `GET /instance/:id/qr` or `GET /instance/:id/qrcode`.
3. Pair the real WhatsApp device.
4. Poll:
   - `GET /instance/:id/status`
   - `GET /instance/:id/runtime`
   - `GET /instance/:id/runtime/history`

Expected:

- QR or pairing state is returned without generic `500`.
- Device pairing completes.
- Status moves toward connected/open when the bridge confirms the session.
- Runtime history records meaningful lifecycle events such as pairing, connected, status observed, disconnect, or reconnect events.

Log observation:

- Look for one request log per operator request.
- Look for lifecycle action/result logs for pairing.
- Confirm there is no repeated QR/pairing log burst while the operator is idle.

Persistence checks:

- `GET /instance/:id/runtime` should show durable state after observed runtime events.
- `GET /instance/:id/runtime/history` should include pairing or status events after successful interaction.

What worked:

- Not executed in this session.

What failed:

- Not executed in this session.

Partial but acceptable:

- Live runtime fields remain bridge-dependent.
- Durable runtime history is only as complete as events observed by this SaaS process.

Must fix before real clients:

- Any generic `500` during normal pair/connect/reconnect conditions.
- Pairing loops that generate continuous QR or status logs.
- Runtime history missing all lifecycle events after a successful real pairing.

## Reconnect Stability

Steps:

1. With the instance paired, call `POST /instance/:id/reconnect`.
2. Watch logs for two minutes.
3. Call status/runtime/history endpoints again.
4. Repeat once after a short wait.

Expected:

- Reconnect returns an operator-facing compatibility envelope.
- No tight loop or repeated reconnect spam appears in logs.
- Runtime history records the reconnect request and subsequent observed status when available.
- CPU and memory remain stable.

If the instance disconnects:

1. Stop sending messages and broadcasts.
2. Call `GET /instance/:id/runtime` and save the response.
3. Call `GET /instance/:id/runtime/history?limit=20` and save the response.
4. Call `POST /instance/:id/reconnect` once.
5. Wait at least 60 seconds before retrying.
6. If reconnect fails twice, stop RC1 and record the error, request ID, tenant ID, instance ID, and last runtime history entries.

Do not repeatedly spam reconnect. Repeated reconnect calls can hide the original failure and create misleading runtime history.

Findings:

- Not executed in this session.

Blockers:

- Any reconnect request that creates duplicate runtime sessions for the same instance.
- Any runaway reconnect loop.
- Any log burst that continues without operator activity.

## Messaging Validation

### Text Send

Steps:

1. Send a text message with `POST /instance/:id/messages/text`.
2. Poll `GET /instance/:id/messages/text/:jobID`.
3. Check recipient device.
4. Query `POST /instance/:id/messages/search` for the recipient chat.

Expected:

- Initial response is accepted/queued.
- Job progresses through queued/running/sent where the bridge returns send evidence.
- Delivered/read are best-effort and may lag or remain absent.
- Outbound message appears in tenant-scoped message history.

Failure signals:

- Job remains `queued` or `running` beyond the expected bridge timeout.
- Recipient receives more than one message for one accepted job.
- API returns a generic `500` instead of validation/conflict/timeout/rate-limited error.
- Logs show repeated send attempts without a new operator action.

### Media Send

Steps:

1. Send a small supported media payload with `POST /instance/:id/messages/media`.
2. Confirm recipient receives the media.
3. Confirm outbound history captures media metadata and caption when present.

Expected:

- No sensitive media payload data is logged.
- Send result includes confirmable send evidence when the bridge provides it.

Failure signals:

- Media payload or URL appears in logs.
- Unsupported payloads return unclear errors.
- Recipient receives duplicate media.

### Audio Send

Steps:

1. Send a supported audio payload with `POST /instance/:id/messages/audio`.
2. Confirm recipient receives playable audio.
3. Confirm status/job behavior stays consistent with text/media expectations.

Expected:

- Audio conversion/runtime send path does not leak payload data in logs.
- Failures return explicit validation/runtime errors.

Failure signals:

- Audio conversion failure causes repeated retries without operator action.
- Audio payload data appears in logs.
- API process CPU remains elevated after the request completes.

Findings:

- Text send: not executed in this session.
- Media send: not executed in this session.
- Audio send: not executed in this session.

Partial but acceptable:

- Delivered/read progression remains best-effort and runtime-receipt dependent.
- Message history is the SaaS-observed history, not universal WhatsApp history.

Must fix before real clients:

- Duplicate sends for a single accepted text job.
- Missing outbound history for successful sends.
- Unclear or generic errors for unsupported media/audio payloads.
- Sensitive payloads, tokens, or URLs in logs.

## Broadcast Validation

Setup:

- Create two or three real opted-in CRM contacts.
- Associate contacts with the test instance or leave them tenant-wide.
- Use a low-risk test message.

Steps:

1. Create a broadcast with `POST /broadcast`.
2. Poll `GET /broadcast/:id`.
3. Poll `GET /broadcast/:id/recipients`.
4. Confirm recipients receive at most one message each.
5. Introduce one intentionally invalid or unreachable recipient only if safe to do so.

Expected:

- Recipient snapshot is stable for the job.
- No duplicate sends to the same phone.
- Recipient progress moves from pending to sent when send evidence exists.
- Permanent recipient failures do not block valid recipients.
- Retryable failures pause and resume from pending recipients only.
- Job may complete as `completed_with_failures` when some recipients fail permanently.

Safe RC1 broadcast settings:

- Audience: two or three opted-in contacts.
- Message: clearly marked test message.
- `rate_per_hour`: low enough to make sends easy to observe.
- Wait for recipient progress before creating another broadcast.

Duplicate-send detection:

- Compare `GET /broadcast/:id/recipients` with recipient devices.
- Each recipient should receive at most one message for the job.
- If any duplicate appears, stop broadcast testing immediately and record the broadcast ID, recipient phone, message IDs, and logs around the duplicate.

Findings:

- Not executed against a real audience in this session.

Partial but acceptable:

- Delivered/read progress appears only when runtime receipts can be matched by durable instance/message identifiers.
- Recipient display enrichment is currently limited; progress is keyed by stored recipient rows.

Must fix before real clients:

- Duplicate sends during retry/resume.
- Broadcast job stuck forever with no retry/failure state.
- Recipient progress contradicting observed sends.
- Failure on one recipient preventing all other valid recipients from completing.

## Runtime Observation

Observe during all real tests:

- API process CPU
- API process memory
- PostgreSQL connection count
- Log volume per action
- Repeated warnings/errors
- Broadcast worker queue behavior
- AI worker logs if AI is enabled

Expected:

- No sustained CPU spike after actions complete.
- Memory does not grow steadily during idle observation.
- No infinite loops.
- No repeated logs without new operator/user activity.
- Logs include enough context: `request_id`, `tenant_id`, and `instance_id` where applicable.

How to observe logs:

```bash
tail -f rc1-api.log
```

Useful filters:

```bash
grep -E '"level":"(WARN|ERROR)"|broadcast|pair|reconnect|runtime|send' rc1-api.log
grep -E 'request_id|tenant_id|instance_id' rc1-api.log | tail -n 50
```

How to detect failure:

- Same warning/error repeats continuously while no requests are being made.
- New logs continue for the same `instance_id` after all RC1 actions stop.
- CPU remains elevated for more than two minutes after the last request.
- Memory grows continuously during a 10-minute idle window.
- PostgreSQL connection count climbs and does not settle.
- Runtime history stops updating after real pairing/reconnect events.
- Broadcast recipient progress does not change despite confirmed recipient delivery.

What to do on failure:

1. Stop creating new sends or broadcasts.
2. Save the last 200 log lines.
3. Capture `/readyz`, `/instance/:id/runtime`, and `/instance/:id/runtime/history?limit=20`.
4. Capture the relevant send job or broadcast recipient endpoint.
5. If CPU/log volume is still rising, send `SIGTERM` and verify graceful shutdown.
6. Record the failure under the relevant findings section in this document.

Temporary observability logs:

- No additional temporary logs were added for RC1 in this pass.
- Existing logs should be sufficient for first real-instance validation.
- Add temporary debug logs only if a real test produces ambiguous runtime behavior, and remove or downgrade them before real clients.

Minimal acceptable RC1 evidence:

- `rc1-api.log` covering startup through shutdown.
- Screenshots or copied responses for `/livez`, `/healthz`, and `/readyz`.
- Runtime/history response after pairing.
- Text send job response and status response.
- Message history response showing the outbound text.
- Broadcast detail and recipient progress responses.
- Process CPU/memory sample before testing, during broadcast, and after 10 minutes idle.

## RC1 Results Summary

What worked:

- Build path for `cmd/api` passed in this workspace.
- Startup configuration fails fast with clear messages for invalid deployment env.
- RC1 real-instance validation checklist is now documented.

What failed:

- Real WhatsApp pairing, sends, and broadcast were not executed in this session because no real deployment database, API runtime, authenticated tenant, or WhatsApp device was provided.

Partial but acceptable:

- Runtime truth remains partly bridge-dependent.
- Delivered/read status progression is best-effort.
- Message history is SaaS-observed, not complete WhatsApp archive parity.
- Broadcast receipt progression depends on matched runtime receipt events.

Must fix before real clients:

- Any duplicate sends observed in text or broadcast testing.
- Any persistent reconnect or broadcast worker loop.
- Any sensitive value appearing in production logs.
- Any normal pairing/reconnect condition returning a generic `500`.
- Any broadcast job stuck without terminal status, retry scheduling, or clear operator error.

## Verdict

Ready for RC1 real-world validation: yes.

Ready for real clients: not yet verified.

The backend is prepared for controlled RC1 validation with a real WhatsApp instance and a very small opted-in audience, but a ready-for-clients verdict requires completing the real pairing, messaging, broadcast, and runtime observation steps above and recording the results here.
