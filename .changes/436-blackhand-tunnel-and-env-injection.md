### feat(tunnel): supervise a named Cloudflare Tunnel and inject settings from the environment

The gateway could not expose itself on a stable hostname. The Cloudflare tunnel
route was a stub that returned 400, and the only Tailscale path required a
daemon on the host. A named tunnel — the one Cloudflare gives a fixed hostname
and no rotating `trycloudflare.com` URL — was not reachable at all.

`internal/tunnel/cloudflared` supervises the `cloudflared` process: it
materializes an inline tunnel config and credentials JSON into the data
directory with mode 0600, starts the tunnel, waits for the readiness line on
the child's output, resolves the public hostname from the config's ingress
entry, and relaunches the tunnel with bounded backoff when the process exits
on its own. A deliberate stop suppresses that relaunch, so a disabled tunnel
does not come back on its own. The mode is read from the environment and
defaults to `off`, so the feature is opt-in: `CF_TUNNEL_MODE=named` with
`CF_TUNNEL_CONFIG` / `CF_TUNNEL_CREDENTIALS` (or the `_FILE` variants)
enables a named tunnel, and `CF_TUNNEL_MODE=quick` falls back to a quick
tunnel with `CF_TUNNEL_ORIGIN_URL`.

`internal/bootinject` applies environment-driven configuration to the
database-backed settings and API keys at startup, so a runtime with an
ephemeral data directory — a Colab notebook, a container — reproduces itself
from environment variables instead of dashboard clicks. API keys come from
`ROUTER_API_KEY`, `ROUTER_API_KEYS` (comma-separated) or `ROUTER_API_KEYS_JSON`
(an array of strings or of `{key, name}` objects); settings come from
`ROUTER_SETTINGS_JSON` plus flat variables such as `ROUTER_REQUIRE_API_KEY`,
`ROUTER_RTK_ENABLED` and `ROUTER_CAVEMAN_LEVEL`. The injection is idempotent:
a key that is already present is left alone, so a re-run never duplicates it
and never overwrites an edit made in the dashboard. An invalid payload is
logged and skipped rather than applied partially.

`GET /api/tunnel/status` now reports the live state of the supervised process
instead of deriving "running" from the settings row, and adds the tunnel mode
and whether one is configured at all, so the dashboard can say "not
configured" rather than failing opaquely. `POST /api/tunnel/disable` stops the
supervised process; before, it only cleared the settings row and left the
tunnel serving traffic. The new `POST /api/tunnel/cloudflare-enable` and
`/api/tunnel/cloudflare-disable` routes drive the supervisor from the
dashboard.

The dashboard's Tunnel row now shows the configured mode, offers the Enable
button only when a tunnel is actually configured, and explains which
environment variables to set when it is not.

The binary is named `blackhand` and the web app is titled Blackhand. The Go
module path, the provider identifiers and the API routes are unchanged, so
upstream merges stay mechanical.

Four defects surfaced while verifying the above, and are fixed here:

- The injection was applied from the router constructor, which runs after the
  token-saver configuration has already been built from the settings row. The
  injected value was stored and ignored: the row said `rtkEnabled=false` while
  the process still had RTK on. The injection moved to its own fx module,
  declared between the database and the server, so it runs before the first
  reader of the row.
- The tunnel was started with the context of the HTTP request that enabled it.
  net/http cancels that context when the handler returns, so the process was
  killed the moment the tunnel was enabled - the enable response reported a
  pid that was already gone, and the tunnel only survived because the
  supervisor relaunched it.
- The tunnel process was not stopped when the gateway exited. It was
  reparented to init and kept running: the tunnel hostname stayed routed to a
  port nothing was listening on, and a runtime that restarts regularly
  accumulated one stale connector per restart. The supervisor is now stopped
  from a shutdown hook.
- The tunnel gate refuses a login or a dashboard page that arrives through the
  tunnel's own hostname unless dashboard access is enabled there. A
  deployment whose only way in is the tunnel therefore could not reach its
  dashboard at all, and the symptom was a 403 that looked like a wrong
  password. `ROUTER_TUNNEL_DASHBOARD_ACCESS` opens it, and the Colab
  bootstrap sets it.

The Colab bootstrap resolves a runnable binary rather than assuming a
toolchain: Colab has no Go and no Bun, and the binary embeds the built web
app, so the runtime cannot compile the gateway. It takes a binary the operator
placed there, a cached build, a prebuilt linux/amd64 release, or - as a
fallback - a source build that installs Go and Bun first.

Verified with unit tests over the supervisor (readiness detection, readiness
timeout, hostname resolution, file materialization, explicit-path precedence,
relaunch after an unexpected exit, no relaunch after a deliberate stop, and
survival past the context that started it), over the injection (seeding,
idempotency, sugar variables, invalid payload, dashboard access), and over the
dashboard handlers (status reflects the live process, disable stops it). A live
run seeded an API key and the settings from the environment, applied them
before the token-saver configuration was built, started a named tunnel on the
hostname declared in the tunnel config, logged in through the tunnel's own
hostname, and confirmed the tunnel process stops with the gateway.
