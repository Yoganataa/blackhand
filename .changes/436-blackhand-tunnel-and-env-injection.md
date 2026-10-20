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

Verified with unit tests over the supervisor (readiness detection, readiness
timeout, hostname resolution, file materialization, explicit-path precedence,
relaunch after an unexpected exit, no relaunch after a deliberate stop), over
the injection (seeding, idempotency, sugar variables, invalid payload), and
over the dashboard handlers (status reflects the live process, disable stops
it). A live run seeded an API key and three settings from the environment,
started a named tunnel on the hostname declared in the tunnel config, and
relaunched the tunnel after its process was killed.
