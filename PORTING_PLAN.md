# Porting Plan: 9Router → Blackhand (Go + Svelte)

## Goal and boundary

Port the requested management/runtime areas while keeping OpenCode Free as the only initial provider:

- Endpoint & Key
- Providers: OpenCode Free only, public model prefix `oc/`
- Usage
- Token Saver
- Console Log
- Settings
- Proxypools

The branch is a feature port, not a promise of 1:1 parity or production readiness. Compatibility claims should be backed by tests against a pinned upstream 9Router revision and representative database copies.

## Current status

| Area | Status | Implemented | Remaining gap |
|---|---|---|---|
| Runtime / packaging | Implemented baseline | Go `net/http`, readiness/liveness, graceful shutdown, embedded Svelte SPA, one-binary smoke test | OS/architecture release matrix, load and lifecycle tests |
| Endpoint & Key | Partial | Key CRUD, environment key reconciliation, active flag, per-key model allow-list, `/v1/models`, `/v1/chat/completions` | Full endpoint parity, more malformed-input and concurrent-key tests, comparison against existing installations |
| OpenCode Free | Partial | Dynamic model discovery with TTL cache, `oc/` prefix normalization, streaming pass-through, client cancellation propagation, optional outbound proxy | No OAuth/account management or other providers; audit upstream errors/retries and stream disconnect semantics |
| Settings | Partial | Persisted JSON settings, defaults, unknown-key preservation, password update | Password login only; OIDC/SAML values are stored but their login flows are not implemented; some settings are placeholders |
| Usage | Implemented baseline; concurrency-tested | Request persistence and daily rollups; stress test covers 192 concurrent writes with a concurrent history reader; 5,000-row history query verifies limit, timestamp index and SQLite integrity | Exact cost/account aggregation parity, retention/export, longer stream tests, larger workload and memory benchmarks |
| Token Saver | Partial; one filter fixture-backed | A subset of RTK output compaction; build-output output is byte-compared with a fixture derived from pinned 9Router source; optional Headroom timeout/fail-open circuit | Other filters and prompts/order are not fully fixture-proven; PXPipe is deliberately skipped; Headroom deployment/bootstrap is external |
| Console Log | Partial | Bounded in-memory buffer, server/request events, SSE, clear/reconnect behavior | No persistence, complete translator/provider events, or exact UI parity |
| Proxypools | Partial | CRUD, status test, active/strict options, no-proxy matching, standard HTTP(S) transport, Vercel/Cloudflare/Deno/Netlify relay headers | Relay deployment automation, per-provider-connection selection/rotation, all upstream edge cases and strict/no-proxy fixtures |
| SQLite compatibility | Synthetic migration/restore-tested; real DB pending | Fixture derived from the upstream schema shape at commit `db6112897399ac981ca73ed573d14102e84ca0f1` covers additive v1→v2 migration, data retention, pre-change backup, and restoring the backup into a clean DATA_DIR; unrelated extension table survives | A sanitized copy of an actual 9Router database is still required; verify real keys, usage rollups, settings and proxy data against a real installation |

## Architecture

- Go HTTP server is the runtime and serves API plus the embedded SPA.
- Svelte + Vite is build-time UI tooling; Node.js is not needed at runtime.
- SQLite via `database/sql` and a pure-Go driver.
- All mutable data and backups stay outside the executable under `DATA_DIR`.
- Streaming requests do not use a global write timeout; upstream requests are bounded and follow client cancellation.
- Usage statistics use calendar-day rollups for calendar-day ranges; the rolling 24-hour aggregate remains a timestamp-bounded history query. The recent-request and last-10-minute queries are bounded.
- Timestamps written by Go are normalized to fixed UTC millisecond ISO strings to keep SQLite text-range comparisons consistent with the upstream JavaScript `toISOString()` format.
- API-key values must not appear in logs or statistics response metadata. Dashboard key-management responses remain authenticated and intentionally show managed key values.

## Compatibility rules

1. Keep `/v1/models` and `/v1/chat/completions` response behavior compatible where implemented.
2. Forward SSE incrementally. Never retry after response bytes have reached the client.
3. Bound request bodies, upstream response sizes, timeouts and in-memory log buffers.
4. When a proxy pool is marked strict, do not silently retry directly after the standard proxy transport fails.
5. Back up an existing SQLite database before schema repair. Do not drop unrelated tables/rows because provider areas are out of scope.
6. Keep credentials out of logs and dashboard statistics. Reject or fail closed when a restricted API key's allow-list is corrupt.
7. Treat Token Saver as incomplete until captured upstream fixtures prove filter detection, output transformation, ordering, defaults and bypass behavior.
8. Do not report numeric performance improvements until measured against the JavaScript implementation with identical mock upstreams and payloads.

## Release gates still open

1. Synthetic upstream-shaped schema-v1 fixture now verifies migration, preserved settings/keys/providers/proxy/combos/aliases/pricing/usage/request details, pre-change backup permissions, and restore. Still run the same assertions against a sanitized copy of an actual 9Router database before release.
2. One build-output fixture is byte-compared against 9Router `open-sse/rtk/filters/buildOutput.js` at commit `db6112897399ac981ca73ed573d14102e84ca0f1`; capture equivalent fixtures for every remaining RTK filter, detection order, prompts, defaults and bypass cases.
3. Add edge tests for streaming cancellation, upstream 4xx/5xx bodies, tool-call payload variants, API-key access-policy cases, relay failures, no-proxy and strict mode.
4. CI runs `go test -race ./...`; a stress test covers 192 concurrent usage writes alongside reads, and a 5,000-row history test verifies bounds, the timestamp index and SQLite integrity. Still test longer streams, cross-process write contention, realistic high-cardinality production data and memory use with large history.
5. Add packaging for supported OS/architectures and verify clean-start and upgrade paths.
6. Reconcile each persisted setting and API response with upstream source before removing any compatibility placeholder.

## Verification

CI currently checks:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `npm run check`
- `npm run build`
- Build and smoke-test the executable after removing the intermediate embedded-assets directory

Passing CI demonstrates compilation, unit tests and basic package startup; it is not a substitute for the release gates above.
