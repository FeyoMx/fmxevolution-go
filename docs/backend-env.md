# Backend Environment Variables

This backend currently reads configuration from two sources:

- `internal/config`: the SaaS API (`cmd/api`)
- `pkg/config`: the legacy runtime bridge used for QR, connection state, webhooks, and advanced instance settings

If you run `cmd/api`, you should think in terms of:

1. SaaS API variables: required
2. Legacy bridge variables: required if you want live WhatsApp runtime features

## SaaS API Variables

Loaded by `internal/config/config.go`.

### Required

| Variable | Purpose | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL DSN for the SaaS database | none |
| `JWT_SECRET` | HMAC secret for JWT access/refresh tokens | none |

### HTTP

| Variable | Purpose | Default |
|---|---|---|
| `APP_ENV` | app mode | `development` |
| `HTTP_ADDRESS` | HTTP bind address | `:8080` |
| `HTTP_READ_TIMEOUT` | request read timeout | `15s` |
| `HTTP_WRITE_TIMEOUT` | response write timeout | `15s` |
| `HTTP_SHUTDOWN_TIMEOUT` | graceful shutdown timeout | `20s` |

### Database pool

| Variable | Purpose | Default |
|---|---|---|
| `DATABASE_MAX_OPEN_CONNS` | max open DB connections | `20` |
| `DATABASE_MAX_IDLE_CONNS` | max idle DB connections | `5` |
| `DATABASE_CONN_MAX_LIFETIME` | connection lifetime | `30m` |

### Auth

| Variable | Purpose | Default |
|---|---|---|
| `JWT_TTL` | access token TTL | `24h` |
| `JWT_REFRESH_TTL` | refresh token TTL | `168h` |

### Broadcast

| Variable | Purpose | Default |
|---|---|---|
| `BROADCAST_WORKERS` | worker goroutines | `4` |
| `BROADCAST_QUEUE_BATCH_SIZE` | DB claim batch size | `8` |
| `BROADCAST_RATE_PER_SECOND` | config value currently not enforced directly in service pacing | `2` |

### Rate limiting

| Variable | Purpose | Default |
|---|---|---|
| `RATE_LIMIT_BACKEND` | rate-limit backend selector | `memory` |
| `RATE_LIMIT_MESSAGES_PER_MINUTE` | available helper policy | `60` |
| `RATE_LIMIT_BROADCAST_PER_HOUR` | broadcast route limit | `120` |
| `RATE_LIMIT_WEBHOOK_CALLS_PER_MINUTE` | webhook dispatch limit | `120` |

### AI

| Variable | Purpose | Default |
|---|---|---|
| `OPENAI_API_KEY` | API key for OpenAI-compatible completion calls | empty |
| `OPENAI_BASE_URL` | base URL for OpenAI-compatible API | `https://api.openai.com/v1` |
| `OPENAI_MODEL` | default AI model | `gpt-4o-mini` |
| `AI_TIMEOUT` | request timeout for AI calls | `15s` |
| `AI_WORKERS` | async AI workers | `2` |
| `AI_MEMORY_LIMIT` | number of historical messages loaded into context | `12` |

## Legacy Runtime Bridge Variables

Loaded by `pkg/config/config.go`.

These remain relevant because `cmd/api` initializes `internal/instance/LegacyRuntime`, which in turn depends on legacy runtime configuration for:

- QR codes
- connect / disconnect
- status snapshots
- instance tokens
- webhook delivery from WhatsApp events
- advanced instance settings such as `ignoreGroups` and `ignoreStatus`

### Legacy database connectivity

| Variable | Purpose |
|---|---|
| `POSTGRES_AUTH_DB` | legacy auth DB DSN |
| `POSTGRES_USERS_DB` | legacy users DB DSN |
| `POSTGRES_HOST` | fallback host if DSNs are not provided |
| `POSTGRES_PORT` | fallback port |
| `POSTGRES_USER` | fallback user |
| `POSTGRES_PASSWORD` | fallback password |
| `POSTGRES_DB` | fallback database name |

### Legacy runtime core

| Variable | Purpose |
|---|---|
| `GLOBAL_API_KEY` | legacy global API key |
| `CLIENT_NAME` | runtime client name |
| `OS_NAME` | runtime OS label |
| `CONNECT_ON_STARTUP` | auto connect legacy instances on startup |
| `WEBHOOK_FILES` | include file payloads in webhook processing |

### Legacy webhook/event backends

| Variable | Purpose |
|---|---|
| `WEBHOOK_URL` | global legacy webhook target |
| `AMQP_URL` | RabbitMQ connection |
| `AMQP_GLOBAL_ENABLED` | enable global AMQP publishing |
| `AMQP_GLOBAL_EVENTS` | AMQP global event allowlist |
| `AMQP_SPECIFIC_EVENTS` | AMQP specific event allowlist |
| `NATS_URL` | NATS connection |
| `NATS_GLOBAL_ENABLED` | enable global NATS publishing |
| `NATS_GLOBAL_EVENTS` | NATS global event allowlist |

### Legacy message filtering

| Variable | Purpose |
|---|---|
| `EVENT_IGNORE_GROUP` | global ignore-groups toggle |
| `EVENT_IGNORE_STATUS` | global ignore-status toggle |
| `QRCODE_MAX_COUNT` | QR generation retry cap |
| `CHECK_USER_EXISTS` | user-existence precheck |

### Legacy media / infra helpers

| Variable | Purpose |
|---|---|
| `MINIO_ENABLED` | enable MinIO/S3 integration |
| `MINIO_ENDPOINT` | MinIO/S3 endpoint |
| `MINIO_ACCESS_KEY` | MinIO/S3 access key |
| `MINIO_SECRET_KEY` | MinIO/S3 secret key |
| `MINIO_BUCKET` | bucket name |
| `MINIO_USE_SSL` | SSL toggle |
| `MINIO_REGION` | bucket region |
| `API_AUDIO_CONVERTER` | audio converter endpoint |
| `API_AUDIO_CONVERTER_KEY` | audio converter key |
| `PROXY_HOST` | proxy host |
| `PROXY_PORT` | proxy port |
| `PROXY_USERNAME` | proxy username |
| `PROXY_PASSWORD` | proxy password |
| `WHATSAPP_VERSION_MAJOR` | optional WA version pin |
| `WHATSAPP_VERSION_MINOR` | optional WA version pin |
| `WHATSAPP_VERSION_PATCH` | optional WA version pin |

### Logging / bootstrap

| Variable | Purpose |
|---|---|
| `DEBUG_ENABLED` | legacy runtime debug mode |
| `LOG_TYPE` | legacy log type |
| `LOG_DIRECTORY` | legacy log directory |
| `LOG_MAX_SIZE` | rolling log max size |
| `LOG_MAX_BACKUPS` | rolling log backup count |
| `LOG_MAX_AGE` | log retention |
| `LOG_COMPRESS` | compress old logs |
| `RESET_ADMIN_PASSWORD` | dev/test password reset helper used by `cmd/api` bootstrap |

## Consistency Notes

### What was stale before

The old root `README.md`, old changelog, and previous env examples described a legacy public API using variables such as `SERVER_PORT`, while `cmd/api` actually requires:

- `DATABASE_URL`
- `JWT_SECRET`
- `HTTP_ADDRESS`

### What is still partial

- The app uses GORM `AutoMigrate`; it does not automatically execute `migrations/000001_saas_core.sql`
- Some runtime behavior still depends on legacy variables that are not part of `internal/config`
- If the legacy runtime cannot initialize, the SaaS API still boots, but QR/connect/status/advanced-settings integration becomes partial or unavailable

## Recommended Practice

- Use `.env.example` at the repo root as the primary starting point for `cmd/api`
- Treat `docker/examples/.env.example` as the Docker-oriented version of the same setup
- Keep both the SaaS database config and the legacy bridge config present when you expect real WhatsApp runtime behavior
