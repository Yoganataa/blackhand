# Blackhand — 9Router Go Port

Blackhand is a Go-first, single-executable porting project for the selected 9Router feature set. Initial provider scope is OpenCode Free (`oc/`).

## Status

This repository currently contains the first architecture scaffold, not a claim of full 1:1 parity. It includes Go server foundations, embedded SPA assets, an OpenCode-compatible model listing and chat proxy, an optional environment API-key gate, a SQLite storage foundation, a Svelte dashboard shell, and CI.

Full legacy SQLite import/export and schema parity, complete API-key CRUD, usage aggregation, every Token Saver algorithm, complete proxy-pool CRUD, and exhaustive legacy API parity remain follow-up milestones. They must be implemented against the selected 9Router source revision and compatibility tests rather than guessed.

## Requirements

- Go 1.23+
- Node.js 20+ and npm for frontend development/build only
- SQLite via pure-Go driver (no runtime CGO requirement)

## Develop

```sh
make dev
```

Go serves API on port 20128; Vite serves UI on port 5173 and proxies `/api` and `/v1` to Go.

## Build one executable

```sh
make build
./bin/blackhand
```

The frontend is compiled into `web/dist`, copied into `internal/httpapi/assets`, and embedded in the Go executable. Production runtime needs no Node.js. Data persists outside the executable under `DATA_DIR` (default `./data`).

Environment variables: `HOST` (default `127.0.0.1`), `PORT` (default `20128`), `DATA_DIR`, `NINEROUTER_API_KEY` (protects `/v1/*` when set), `OPENCODE_MODELS_URL`, `UPSTREAM_BASE_URL`, and optional `UPSTREAM_API_KEY`.

See [PORTING_PLAN.md](./PORTING_PLAN.md) for the compatibility policy and milestones.
