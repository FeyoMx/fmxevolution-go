# Backend Deployment

This backend is deployable as a single API process on a VPS or in a container. It requires PostgreSQL and should run behind a TLS-terminating reverse proxy such as Nginx, Caddy, Traefik, or a platform load balancer.

## Build

```powershell
go build -mod=readonly -o api.exe ./cmd/api
```

Linux/container builds can use the same package target:

```bash
go build -mod=readonly -o api ./cmd/api
```

## Required Environment

The API fails fast when any required deployment variable is missing or invalid:

- `DATABASE_URL`: PostgreSQL connection string.
- `JWT_SECRET`: signing secret for access and refresh tokens. Use at least 32 characters in production.
- `PORT` or `HTTP_ADDRESS`: TCP bind config. Prefer `PORT=8080` for containers/platforms; existing VPS deployments can keep `HTTP_ADDRESS=:8080`.

Recommended production variables:

- `APP_ENV=production`
- `LOG_LEVEL=info`
- `HTTP_READ_TIMEOUT=15s`
- `HTTP_WRITE_TIMEOUT=15s`
- `HTTP_SHUTDOWN_TIMEOUT=20s`
- `DATABASE_MAX_OPEN_CONNS=20`
- `DATABASE_MAX_IDLE_CONNS=5`
- `DATABASE_CONN_MAX_LIFETIME=30m`
- `RATE_LIMIT_BACKEND=memory`
- `RATE_LIMIT_MESSAGES_PER_MINUTE=60`
- `RATE_LIMIT_BROADCAST_PER_HOUR=120`
- `RATE_LIMIT_WEBHOOK_CALLS_PER_MINUTE=120`
- `BROADCAST_WORKERS=4`
- `BROADCAST_QUEUE_BATCH_SIZE=8`

Optional AI variables:

- `OPENAI_API_KEY`
- `OPENAI_BASE_URL`
- `OPENAI_MODEL`
- `AI_TIMEOUT`
- `AI_WORKERS`
- `AI_MEMORY_LIMIT`

## Run On A VPS

1. Provision PostgreSQL and create the application database.
2. Build the binary with `go build -mod=readonly -o api ./cmd/api`.
3. Export the required environment variables.
4. Start the API:

```bash
APP_ENV=production \
LOG_LEVEL=info \
DATABASE_URL='postgres://user:password@127.0.0.1:5432/fmx?sslmode=disable' \
JWT_SECRET='replace-with-at-least-32-random-characters' \
HTTP_ADDRESS=:8080 \
./api
```

5. Put a reverse proxy in front of `http://127.0.0.1:8080`.
6. Configure the process manager, such as systemd, Docker, or your VPS panel, to send `SIGTERM` on stop and allow at least `HTTP_SHUTDOWN_TIMEOUT` for graceful shutdown.

## Run In A Container

Expose the configured `PORT` and pass environment variables through the container runtime:

```bash
docker run --rm \
  -e APP_ENV=production \
  -e LOG_LEVEL=info \
  -e DATABASE_URL='postgres://user:password@db:5432/fmx?sslmode=disable' \
  -e JWT_SECRET='replace-with-at-least-32-random-characters' \
  -e PORT=8080 \
  -p 8080:8080 \
  fmx-backend:latest
```

## Health Checks

- `GET /livez`: process is alive.
- `GET /healthz`: basic API health.
- `GET /readyz`: database readiness check.

Suggested container checks:

- Liveness: `GET /livez`
- Readiness: `GET /readyz`

Only `/healthz`, `/livez`, `/readyz`, `/auth/login`, `/auth/refresh`, and `/tenant` are intentionally public. Application routes are protected by auth and tenant middleware.

## Logging

Logs are JSON to stdout. Set `LOG_LEVEL` to one of:

- `debug`
- `info`
- `warn`
- `error`

Use `info` or `warn` for production unless actively troubleshooting. The API avoids logging request bodies, secrets, access tokens, refresh tokens, API keys, JWT secrets, and webhook signing secrets.

## Shutdown

The API handles `SIGINT` and `SIGTERM`.

Shutdown order:

1. Stop accepting HTTP requests.
2. Cancel background worker contexts.
3. Wait for broadcast and AI workers to exit or for `HTTP_SHUTDOWN_TIMEOUT`.
4. Close database connections.

## Troubleshooting

- Missing env at startup: check the error for the exact missing variable name.
- `PORT must be an integer between 1 and 65535`: set `PORT` to a plain number such as `8080`, or use `HTTP_ADDRESS=:8080` for legacy VPS-style binds.
- `/readyz` returns `503`: the process is alive, but PostgreSQL is not reachable or credentials are invalid.
- Authenticated routes return `401`: provide a valid bearer token, tenant API key, or supported legacy instance token.
- Authenticated routes return `403`: the token is valid, but the tenant or role does not allow the requested operation.
- Startup fails during database migration: verify PostgreSQL permissions allow schema changes for the application database.
