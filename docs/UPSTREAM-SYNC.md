# Syncing with upstream

Blackhand is a rebase of [`luqman-v1/9router-go`](https://github.com/luqman-v1/9router-go)
(`upstream`). Our commits sit on top of upstream's, so pulling upstream in
means replaying ours on top of a newer upstream — never merging two histories.

```bash
scripts/pull-upstream.sh
```

The script fetches `upstream`, rebases `blackhand` onto `upstream/main`, and
then builds and tests. If a conflict appears it stops and tells you what to do.

## Why the rebase stays small

Everything Blackhand adds lives in files upstream does not touch, and the few
places where a Blackhand change has to sit inside an upstream file are kept to
a single line at a quiet anchor point:

| Blackhand change | Where it lives | Why it is cheap to merge |
| --- | --- | --- |
| Named Cloudflare Tunnel | `internal/tunnel/cloudflared/` | New package. Upstream has no such directory. |
| Environment injection | `internal/bootinject/` | New package. |
| Boot-inject wiring | `internal/app/bootinject.go` + one line in `app.go` | New file; the line in `app.go` sits in the module list. |
| Tunnel HTTP handlers | `internal/handlers/dashboard/cloudflare_tunnel.go` | New file. |
| Route registration | one call at the **end** of each route table | Upstream edits the middle of those tables; an insertion at the end lands in a different hunk and merges cleanly. |
| Dashboard UI | `web/src/components/EndpointView.svelte` | Upstream touches this file rarely (2 commits in the last 60). |
| API client | `web/src/api/client.ts` | Upstream touches this file often (12 commits in the last 60), but the added methods sit in a quiet region of the `api` object. |
| Binary name, page title | `Makefile`, `web/index.html` | The only genuinely conflicting edits: upstream also renames these. Resolve by keeping Blackhand's. |

## Resolving a conflict

Almost every conflict is "upstream added a line next to ours". The rule:

* **Both sides are wanted.** Upstream added a route, we added a route. Keep
  both lines.
* **One side is a brand string.** `Makefile` (`BINARY_NAME`) and
  `web/index.html` (`<title>`): keep Blackhand's version. Upstream renamed
  theirs, and the whole point of the rebrand is that ours wins.

After resolving:

```bash
git add <file>
git rebase --continue
```

If the rebase goes wrong:

```bash
git rebase --abort
```

## Verifying a pull

The script already builds and runs the Go tests. Two more checks are worth
running by hand after a pull that touched the frontend or the route tables:

```bash
cd web && bun run build          # the SPA is embedded into the binary
bun run ratchet:svelte           # unresolved identifiers fail the build
```

The route tables are the one place where a pull can go wrong quietly: a route
registered in the test table but not in the one the server mounts answers 404
in production while every handler test passes. The upstream suite pins this
for its own routes; when you add a route, register it in both.

## What Blackhand changes about upstream

The Go module path stays `9router/proxy`, and the provider identifiers and API
routes stay as they are. That is deliberate: it keeps the database
(`~/.9router/db/data.sqlite`) and the SQLite schema compatible with upstream,
and it keeps a pull mechanical — upstream's diff applies to a tree Blackhand
only renamed in a handful of places.

## Running on Google Colab

Open `colab/Blackhand.ipynb` in Colab. It asks for three things — a dashboard
password, your tunnel `config.yml`, and your tunnel credentials JSON — and
then starts the gateway and prints the endpoint to use.

Colab has no Go toolchain and no Bun, and the gateway binary embeds the built
web app, so the runtime cannot compile the gateway itself. The bootstrap
resolves a binary in this order:

1. `BLACKHAND_BINARY` — a path to a binary placed there already.
2. A cached build from a previous run, in `/content/blackhand-cache`.
3. A prebuilt `linux/amd64` release, from `BLACKHAND_BINARY_URL` or from the
   fork's release page.
4. A source build, which installs Go and Bun first. Slow, and it needs no
   release to exist.

Steps 1-3 are the fast paths. A fork that has not cut a release yet pays a
full build on every fresh runtime, so cutting one is worth it. The bootstrap
also installs `cloudflared`, which Colab does not ship.

```python
import os
os.environ["JWT_SECRET"] = "..."
os.environ["INITIAL_PASSWORD"] = "..."
os.environ["ROUTER_API_KEY"] = "sk-..."
os.environ["CF_TUNNEL_MODE"] = "named"
os.environ["CF_TUNNEL_CONFIG_FILE"] = "/content/config.yml"
os.environ["CF_TUNNEL_CREDENTIALS_FILE"] = "/content/credentials.json"
```

```bash
!git clone https://github.com/Yoganataa/9router-go.git
%cd 9router-go
!python3 scripts/colab-bootstrap.py
```

The data directory is ephemeral, so the environment is the only durable
state: `ROUTER_API_KEY` and `ROUTER_SETTINGS_JSON` reproduce the API key and
the settings on every fresh runtime, and the tunnel material is either
written inline into the environment or read from a file in `/content`.

The bootstrap writes `status.json` next to the data directory, with the
tunnel URL and the endpoint, so a notebook can read the result without
parsing the log.

`ROUTER_TUNNEL_DASHBOARD_ACCESS` is set to true by the bootstrap whenever a
tunnel is configured. The tunnel gate refuses the dashboard on the tunnel's
own hostname otherwise, and on Colab the tunnel is the only way in — without
it the dashboard answers 403 on a password that is correct. The password is
still required: `requireLogin` defaults to on, and this setting only lifts
the hostname gate.
