# Blackhand — selected 9Router port (Go + Svelte)

Blackhand is a Go-first port of a selected part of 9Router. The initial upstream provider is OpenCode Free, exposed with the public model prefix `oc/`. The production server and compiled Svelte SPA are bundled into one executable.

> **Status: work in progress, not production-ready.** CI checks Go tests/vet, Svelte check/build, and an executable smoke test. These checks do not prove full 9Router compatibility, high-load capacity, or migration safety against every real-world database.

## Current implementation

| Area | Implemented now | Known limitation |
|---|---|---|
| Runtime | Go HTTP server, graceful shutdown, readiness and liveness endpoints, embedded SPA assets | Cross-platform packaging and load tests remain |
| Endpoint & keys | `/v1/models`, `/v1/chat/completions`, API-key CRUD, active state, restricted model allow-lists, environment key reconciliation | No full legacy route parity; review key-policy behavior before production |
| Provider | OpenCode Free model discovery, `oc/` to upstream model translation, cache/stale discovery behavior, streaming response forwarding | No provider OAuth/account connection UI; other provider APIs and inference endpoints are outside current scope |
| Dashboard auth | Password login with signed HttpOnly session cookie, login throttling, password changes | Password flow only; persisted OIDC/SAML settings do not implement SSO |
| Settings | SQLite-backed settings with upstream-style defaults and preservation of unknown keys | Some settings are stored for compatibility but are not wired to equivalent runtime behavior |
| Usage | Request history, status/token/cost aggregation, daily rollups, recent requests and chart series | Pricing overrides have no full management UI; exact legacy accounting parity needs fixture comparison |
| Token Saver | A tested subset of RTK tool-output filters, optional Headroom call with timeout/circuit breaker, Caveman/Ponytail system prompts | Not algorithm-for-algorithm parity; PXPipe is explicitly skipped and Headroom is not bundled |
| Console Log | Bounded in-memory buffer, SSE stream, HTTP/request events and selected router events | Not a persistent or complete replacement for 9Router's full console logs |
| Proxypools | List/create/update/delete, health test, active/strict flags, no-proxy rules, standard HTTP(S) and relay request routing | No deployment automation for relay services; per-connection pool binding and all legacy semantics are not yet ported |
| SQLite | SQLite-compatible core tables, backup before detected schema repair, import of selected early Blackhand settings/proxy/usage tables | A complete migration audit against copies of production 9Router databases remains mandatory |

## Build and run

Requirements: Go 1.23+ and Node.js 20+ with npm for frontend development/build only.

Development server:

```sh
make dev
```

Build the single executable:

```sh
make build
./bin/blackhand
```

The build compiles the SPA and embeds its static assets in the Go executable. Node.js is not needed to run the packaged binary. Runtime data, the SQLite database, and backups live outside the executable under `DATA_DIR` (default `./data`).

Run the current checks:

```sh
go test ./...
go vet ./...
cd web && npm run check && npm run build
```

## Configuration

| Variable | Purpose |
|---|---|
| `HOST` | Bind address; defaults to `127.0.0.1` |
| `PORT` | HTTP port; defaults to `20128` |
| `DATA_DIR` | Database, machine ID and auth-secret directory |
| `NINEROUTER_API_KEY` | Reconciled into the local key table; protects inference endpoints when API-key enforcement is enabled |
| `REQUIRE_API_KEY` | Optional environment override for the persisted `requireApiKey` setting |
| `INITIAL_PASSWORD` | Set an initial dashboard password before enabling remote access |
| `API_KEY_SECRET` | Key CRC HMAC secret; defaults to the 9Router-compatible default |
| `MACHINE_ID_SALT` | Machine ID salt; defaults to the 9Router-compatible default |
| `OPENCODE_MODELS_URL` | OpenCode model catalogue URL |
| `UPSTREAM_BASE_URL` | OpenCode-compatible upstream base URL |
| `UPSTREAM_API_KEY` | Optional upstream bearer token |
| `MODEL_DISCOVERY_TIMEOUT` | Model catalogue timeout |
| `UPSTREAM_TIMEOUT` | Inference timeout; long generation streams are supported |
| `MODEL_CACHE_TTL` | Model catalogue cache TTL |

Security notes:
- Keep the dashboard on loopback unless remote access is necessary. If exposed remotely, configure an explicit initial password and terminate TLS at a trusted reverse proxy.
- Keep `DATA_DIR` private. It contains API keys, the machine ID and session-signing secret.
- Do not commit real API keys, passwords, proxy credentials, or tunnel credentials.
- The configured API key is deliberately available through authenticated dashboard key management; protect the dashboard session accordingly.

## Compatibility work still required

Before calling this a production-ready replacement, compare behavior with a pinned 9Router `dev` revision and test a copy of a real database. Remaining work includes exact Token Saver fixtures/ordering, complete route and auth parity, per-connection proxy configuration, upstream error and stream-cancellation edge cases, restore testing, race/load testing, and cross-platform release artifacts. Do not infer performance gains from the language choice; benchmark identical workloads first.

See [PORTING_PLAN.md](./PORTING_PLAN.md) for status and release gates.
