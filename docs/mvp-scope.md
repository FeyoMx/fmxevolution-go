# MVP Scope

Audited on 2026-04-06.

This document defines the backend SaaS surface that is considered in-scope for MVP release candidate validation and pilot use.

## Release intent

The current release candidate is intended for:

- tenant-aware backend validation
- manual QA of the supported SaaS routes mounted by `cmd/api`
- controlled pilot usage where operators understand the current bridge-dependent limits

It is not intended to claim full Evolution Manager parity.

## Supported MVP features

- auth and session routes:
  - `GET /healthz`
  - `POST /auth/login`
  - `POST /auth/refresh`
  - `GET /auth/me`
  - `POST /auth/logout`
- tenant-safe tenant bootstrap:
  - `POST /tenant`
  - `GET /tenant`
- tenant-safe instances:
  - create, list, detail, delete
  - advanced settings
  - tenant-safe `:id` and `id/:instanceID` lookup variants where mounted
- runtime controls:
  - `connect`
  - `disconnect`
  - `reconnect`
  - `pair`
  - `logout`
  - `status`
  - `qr` and `qrcode`
- runtime observability:
  - `GET /instance/:id/runtime`
  - `GET /instance/:id/runtime/history`
- text, media, and audio send:
  - SaaS instance routes
  - current compatibility send aliases still used by the frontend
- chat list:
  - live runtime-backed chat search/list routes
- message search and history:
  - tenant-safe message-history search
  - persisted outbound and inbound read model where available
- inbound and backfill ingestion:
  - inbound webhook publishing into conversation history when payload metadata is sufficient
  - bridge-delivered `HistorySync` ingestion
  - tenant-safe history backfill trigger route
- contacts:
  - create, read, update, tags, notes
- broadcast basic surface:
  - create
  - list
  - detail
  - queue validation and worker claiming
- AI settings:
  - tenant settings
  - instance toggles

## Out of scope or intentionally partial

- full delivery-grade broadcast execution
- deep analytics or trustworthy product reporting beyond current basic counters
- Chatwoot
- SQS
- Kafka
- manager integration suites:
  - OpenAI manager CRUD
  - Typebot
  - Dify
  - N8N
  - EvoAI
  - EvolutionBot
  - Flowise
- perfect historical backfill
- full bridge-independent runtime truth
- full persisted chat-list parity with durable labels, previews, and ordering
- every legacy transport/payload variation outside the current frontend-supported send shapes

## Operational expectations

- `GET /healthz` is the root operational probe for environment checks and release validation smoke tests.
- The source of truth for mounted routes is [backend-api.md](/c:/Users/luis_/OneDrive/Documents/DevWork/fmxevolution-go/docs/backend-api.md).
- Runtime control success does not imply full bridge independence; operator UX should treat runtime actions as accepted bridge work unless the durable runtime routes confirm the resulting state.

## MVP acceptance boundary

The MVP release candidate should be considered acceptable when:

- all supported routes authenticate and remain tenant-safe
- runtime lifecycle and runtime history endpoints return clear, honest responses
- text/media/audio send works for the currently supported frontend payloads
- chat and message-history UX are usable within documented bridge limits
- contacts, AI settings, and broadcast basic validation behave consistently
- unsupported suites remain explicitly partial or out of scope instead of silently pretending parity
