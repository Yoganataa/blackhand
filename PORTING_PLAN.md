# Porting Plan: 9Router → Blackhand (Go + Svelte)

## Goals
1. Prioritize performance and operator ergonomics.
2. Serve API and compiled Svelte SPA from one production executable.
3. Preserve external API contracts, environment/configuration semantics, key behavior, and database compatibility where required.
4. Start with OpenCode Free only, under model prefix `oc/`.
5. Back compatibility claims with tests.

## Initial feature scope
- Endpoint & Key
- Providers: OpenCode Free (`oc/`) only
- Usage
- Token Saver
- Console Log
- Setting
- Proxypools

This repository begins as a scaffold. Port completion requires route-by-route, database, token-saver, and streaming parity tests against the selected 9Router source revision.

## Architecture
- Go `net/http`; isolate API, provider, token processing, storage, usage, and logging.
- Svelte + Vite SPA, with lazy route/page loading.
- Go `embed.FS` bundles `web/dist` into the release executable; Node/npm are build-time only.
- SQLite via `database/sql` and an initially pure-Go driver. Benchmark the actual workload before treating the SQLite driver as final.
- Runtime database, backup, and logs remain external under `DATA_DIR`.
- Context cancellation, bounded buffers, upstream timeouts, graceful shutdown, and streaming correctness are mandatory.

## Compatibility matrix

| Area | Preserve | Safe improvements after tests |
|---|---|---|
| Endpoint & Key | Public routes, auth modes, key status, access restrictions, request/response shapes | True readiness after storage init; uniform errors; safe key comparison |
| OpenCode Free | `oc/` model prefix, upstream IDs, discovery and request semantics | TTL cache and stale-if-error; never retry after stream bytes have started |
| Usage | Provider/model/status/token/cost/time fields and historical aggregation | Batch telemetry where safe, bounded live event fan-out |
| Token Saver | Existing algorithm semantics, order, defaults, toggles and bypasses | Modular pipeline, measured performance, fixture-based regression tests |
| Console Log | Existing relevant events and dashboard display | SSE, bounded ring buffer, backpressure, secret redaction |
| Setting | Keys, value types, defaults, environment precedence and persisted values | Atomic updates, preserve unknown settings |
| Proxypools | CRUD fields, URL/type/noProxy/active/strict/test semantics | Explicit test timeout; strict mode cannot silently fall back to direct networking |

## Engineering rules
- Maintain `/v1/models` and `/v1/chat/completions` contracts.
- Forward SSE incrementally, not by buffering an entire generation.
- Cancel upstream requests when clients disconnect.
- Never retry a stream after response bytes have been emitted.
- Bound request size and upstream operations.
- Treat dynamic model discovery separately from persisted provider connections.
- Liveness only means the process is running; readiness requires successful config/storage initialization.
- Back up an existing 9Router database before touching it. Never drop or rewrite unrelated rows because some providers are outside current scope.
- Token Saver must first have fixtures captured from original implementation. On timeout/error, fail-open only where that matches source behavior.
- Logs must redact API keys and OAuth tokens. Full request dumps, if added, must be opt-in with bounded retention.
- Runtime data and backups stay outside executable.

## Milestones and release gates
1. Inventory source revision, routes, env vars, settings defaults, schema, models, saver fixtures and example requests.
2. Runtime foundation: config, logging, embedded assets, health/readiness, graceful shutdown, SQLite init.
3. Endpoint/key + OpenCode Free: auth, models, upstream proxy, streams, cancellation, errors.
4. Settings + proxypools: full CRUD and real transport selection.
5. Usage + console: event flow, persistence, dashboards, bounded memory, redaction.
6. Token Saver parity: existing algorithms/order/toggles, regression fixtures, benchmarks.
7. Release verification: API tests, race tests, load tests, backup/restore, OS/architecture builds.

## Current scaffold gaps
- Full legacy database schema/migrations and import/export.
- Complete API key CRUD/access controls.
- Full usage history/statistics and dashboard data.
- All Token Saver algorithms and exact default behavior.
- Console log parity beyond safe structured runtime logs.
- Proxypool CRUD persistence and provider transport selection.
- Full route/setting/auth UI parity and integration tests.

## Performance validation
Measure against the JavaScript baseline with the same mock upstream, payload sizes, and concurrency. Track proxy overhead p50/p95/p99, time-to-first-byte, memory at idle/load, stream correctness, SQLite write latency, and Token Saver CPU. Do not advertise numeric improvements until measured.
