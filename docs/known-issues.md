# Known Issues

Audited on 2026-04-06.

This document lists the current MVP risks and operational limits that should be understood before pilot use.

## Bridge dependency

- Runtime connect, disconnect, reconnect, pair, logout, QR, and live status still depend on the legacy bridge.
- Durable runtime status reduces bridge dependence for reads, but the optional live view is still bridge-backed.
- A successful runtime action response means the SaaS layer accepted bridge work; it does not guarantee deeper bridge-independent truth.

## Rate-limit behavior

- Chat list/search remains live-runtime-backed and can inherit upstream `429` or similar rate-limit responses.
- Repeated operator refreshes or aggressive frontend polling can degrade runtime UX.
- Current MVP behavior prefers honest failure over pretending the bridge returned a durable empty result.

## Broadcast limitations

- Broadcast queueing, claiming, and validation are implemented.
- Broadcast delivery execution is still a stub and should not be treated as a delivery-grade campaign engine.
- Broadcast routes are in MVP only as a basic operational surface, not as a fully completed send pipeline.

## Sparse dashboard metrics

- Dashboard metrics are still limited.
- Instance counters are meaningful, but several other analytics-style totals remain placeholders.
- The dashboard should not be used as a trustworthy deep reporting surface in MVP.

## Partial media, audio, and history completeness

- Media and audio sending are supported for the currently implemented frontend payload shapes, not every legacy payload variant.
- Replayed historical media does not imply durable SaaS media storage; persisted history focuses on metadata and structured message content.
- Inbound and historical completeness improved through runtime ingestion and `HistorySync`, but the system still cannot perfectly reconstruct all older conversations or session timelines.
- Chat list parity remains partial because there is no full durable chat repository with labels, previews, and ordering parity.

## Release posture

- This release candidate is suitable for manual QA and controlled pilot use inside the documented MVP scope.
- It is not yet suitable to market as full manager parity or fully bridge-independent runtime control.
