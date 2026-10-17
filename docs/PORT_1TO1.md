# 1:1 port checklist — official 9Router → Blackhand

Tracking artifact for the scoped 1:1 port. Update this file in the same commit as each area, and
keep the status column honest: a row is `done` only when its behaviour is verified against the
pinned upstream checkout, not when the file merely exists.

## Upstream pin

| Item | Value |
|---|---|
| Repository | `https://github.com/decolua/9router` (official, not the `Yoganataa/9router` fork) |
| Branch | `master` |
| Commit | `87a74e1bda3677193c6f71a7e2100c90311f0689` |
| Version | `0.5.104` |
| Reference checkout | `C:\Users\Yoganata\Documents\9router-official` (read-only, outside this repo) |

Move the pin deliberately, one commit at a time, and record the new sha here when the port is
re-verified against it. Do not track `master` implicitly.

**One deliberate exception.** For the OpenCode provider's *upstream client behaviour* — the request
headers, endpoints and session id format its gateway classifies on — the working reference is the
`Yoganataa/9router` `dev` branch (v0.5.111, `db611289`), not official `master`. That branch carries the
fixes this port needed (canonical inverted-timestamp session ids, the v2 client fingerprint, the
inference lane for free models) as commits and as its registry transport headers, while official
`master` and the open upstream pull requests (#4883, #4860) do not have them. Structural work — pages,
routes, database shapes — still follows official `master`.

### Why not the fork (`Yoganataa/9router@dev`, v0.5.111)

The two diverge: the fork is 45 commits ahead and 38 behind official `master`, 47 files differ.
Facts that matter for the seven areas below:

- Official RTK filters are `dedupLog`, `gitDiff`, `gitStatus`, `ls`, `searchList`, `tree`
  (plus `applyFilter`, `autodetect`, `caveman`, `cavemanPrompts`, `ponytailPrompt`, `registry`,
  `systemInject`, `constants`). `open-sse/rtk/filters/buildOutput.js` exists only in the fork, so
  the build-output fixture currently in `internal/tokensaver/testdata` was derived from fork commit
  `db6112897399ac981ca73ed573d14102e84ca0f1` and must be re-derived from the official checkout.
- `src/lib/headroom/bootstrap.js` exists only in the fork; official manages Headroom through the
  `/api/headroom/*` routes and `src/lib/headroom`.
- Official `providers/[id]` has `AutoPingScheduleModal.js`; the fork does not.
- A screenshot of a local v0.5.111 instance therefore does not prove official parity. When the two
  disagree, official `master` wins.

## Scope

In scope, 1:1 frontend (rebranded to Blackhand) and backend:

1. Endpoint & Key
2. Providers — OpenCode Free only for now
3. Usage
4. Token Saver
5. Proxy Pools
6. Console Log
7. Setting (upstream labels the page "Settings" in the sidebar; the route is `/dashboard/profile`)

Out of scope until separately requested: Combo & Vision Adapter, Quota Tracker, CLI Tools,
Translator page, Skills, Media Providers, MITM, Basic Chat (hidden upstream), PXPIPE page (hidden
upstream), 9Remote, 9English. Their upstream entries stay absent or hidden in the sidebar — never
replaced by invented menus.

## Surface inventory (official v0.5.104)

| Area | Frontend | Backend routes | Runtime |
|---|---|---|---|
| Shell | `Sidebar.js` 464, `Header.js` 376, `HeaderMenu`, `HeaderLanguage`, `DashboardLayout.js` 123, `Avatar`/`Badge`/`Button`/`Card`/`Drawer`/`ThemeProvider`, Tailwind v4 + `src/app/globals.css` tokens | — | — |
| Endpoint & Key | `EndpointPageClient.js` 1337 + `EndpointRow`, `KeyAccessControls`, `SecurityWarning`, `StatusAlert`, `Tooltip`, `endpointConstants`, `endpointPing` | `keys` (2), `tunnel` (7) | `src/lib/tunnel` (14) |
| Providers | `providers/page.js` 1041, `providers/[id]/page.js` 1991, 15 components (`ConnectionsCard`, `ConnectionRow`, `CooldownTimer`, `ModelRow`, `ModelsCard`, `NoAuthProxyCard`, `CustomConfigCard`, `AddApiKeyModal`, `AddCustomModelModal`, `AutoPingScheduleModal`, `BulkImportCodexModal`, `BulkImportGrokCliModal`, `CompatibleModelsSection`, `EditCompatibleNodeModal`, `ModelAvailabilityBadge`, `utils`) | `providers` (13), `provider-nodes` (3), `models` (8) | repos `connections`, `nodes`, `disabledModels`, `alias` |
| Usage | `usage/page.js` 78 + `UsageChart`, `UsageTable`, `TopModelsChart`, `ProviderBarChart`, `ProviderTopology`, `OverviewCards`, `RequestDetailsTab`, `ProviderLimits/{index,ProviderLimitCard,QuotaProgressBar,QuotaTable,utils}` | `usage` (11: `stats`, `chart`, `history`, `logs`, `providers`, `request-details`, `request-logs`, `stream`, `[connectionId]`, `[connectionId]/codex-reset-credits`, `[connectionId]/claude-reset`) | repos `usage`, `requestDetails`, `pricing` |
| Token Saver | `TokenSaverClient.js` 1037 | `headroom` (6), `pxpipe` (8) | `open-sse/rtk` (15), `src/lib/headroom`, `src/lib/pxpipe` |
| Proxy Pools | `proxy-pools/page.js` 1173 | `proxy-pools` (7: CRUD, `[id]`, `[id]/test`, `vercel-deploy`, `cloudflare-deploy`, `deno-deploy`, `netlify-deploy`) | `src/lib/network` (5) |
| Console Log | `ConsoleLogClient.js` 97 | `translator/console-logs` (2) | `src/lib/consoleLogBuffer` |
| Setting | `profile/page.js` 1703, `app/dashboard/settings/pricing/page.js` 173 | `settings` (4), `auth` (OIDC/SAML config + test), `version` (shutdown) | — |

Sidebar order upstream: Endpoint & Key, Providers, Combo & Vision Adapter, Usage, Quota Tracker,
Token Saver, CLI Tools, Console Log, Translator, Proxy Pools, Skills, then the System group
(Media Providers, 9Remote, 9English) and Settings → `/dashboard/profile`. Basic Chat and PXPIPE are
hidden upstream. Blackhand keeps only the in-scope items, in upstream's order.

Consequences of 1:1 that are not optional: Endpoint & Key carries the tunnel/Tailscale lifecycle,
Setting carries OIDC/SAML/backup/shutdown, Token Saver carries Headroom and PXPIPE process
lifecycle, Proxy Pools carries the four relay deploys and provider binding.

## Phases

Each phase ends with: `go test -race ./...`, `go vet ./...`, `npm run check`, `npm run build`,
`pwsh -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-windows.ps1`, this table updated.

| Phase | Work | Status |
|---|---|---|
| 0 | Reference checkout, surface inventory, pin recorded, 1:1 checklist | done — checkout at `87a74e1` (v0.5.104) |
| 1 | FE foundation: Tailwind v4 + upstream theme tokens, shell (`Sidebar`/`Header`/`DashboardLayout`) in Svelte, `/dashboard/*` routing, only in-scope nav | foundation built and checked; shell not yet wired into `App.svelte` |
| 2 | Setting (`/dashboard/profile`) + Endpoint & Key (incl. tunnel/Tailscale) | Endpoint & Key page ported; Setting ported for Local Mode + Security only. Open: tunnel/Tailscale rows, the shared model picker, and the rest of the profile cards. |
| 3 | Providers — OpenCode Free: list + detail, connections, model enable/disable/alias/custom, proxy pool binding, custom headers | Free Tier list, detail header, no-auth proxy card and custom headers are ported; connections, models card and batch test are open. |
| 4 | Usage + pricing settings page (per-connection limits, topology, request details) | not started |
| 5 | Token Saver (RTK filter parity re-fixtured from official, Headroom/PXPIPE lifecycle) | not started |
| 6 | Proxy Pools (relay deploys, provider/connection binding) | not started |
| 7 | Console Log parity (log events from the full request pipeline) | not started |
| 8 | Real 9Router database migration test, cross-platform packaging, honest gap record | not started |

Order rationale: the shell hosts every page, Setting/Endpoint carry auth and key foundations,
Providers introduces the connection model that Usage and Proxy Pools bind to, then the remaining
areas.

## Phase 1 progress

Verified on this machine: `npm install`, `npm run check` (0 errors, 0 warnings) and `npm run build`
(2.1 s, 74 kB CSS, 82 kB JS, self-hosted Inter + Material Symbols) all pass. The upstream reference
checkout at `C:\Users\Yoganata\Documents\9router-official` is at the pinned commit.

| Item | File | State |
|---|---|---|
| Theme layer ported from upstream `globals.css` | `web/src/app.css` | written, builds |
| Tailwind v4 via `@tailwindcss/vite`; self-hosted Inter + Material Symbols | `web/package.json`, `web/vite.config.ts`, `web/src/main.ts` | written, installs and builds |
| Pre-paint theme + fonts-loaded scripts, Blackhand branding | `web/index.html` | written |
| Router (real `/dashboard/*` URLs), theme store, toast store | `web/src/lib/{router,theme,notifications}.svelte.ts`, `web/src/lib/config.ts` | written, type-checks clean |
| Sidebar, Header, ThemeToggle, HeaderMenu, DashboardLayout | `web/src/shell/*.svelte` | written, checks clean |
| Wire the shell into the app entry (replaces the legacy chrome in `App.svelte`) | `web/src/App.svelte` | not started |

The Go side needs no change for shell routing: `internal/httpapi/api.go staticHandler` already falls
back to `index.html` for non-`/api`, non-`/v1` paths, so `/dashboard/*` deep links work.

Upstream controls deliberately left out of the shell for now, each owned by a later phase — they
must land as working features, not as dead buttons:

- update banner + `ManualUpdatePanel` + `ConfirmModal` (Sidebar) — version routes, Setting phase
- `HeaderMenu` → "Change Log" (`/api/version/changelog`) and "Shutdown" (`/api/version/shutdown`)
- OIDC/SAML identity chip in the Header (`/api/auth/status`)
- `HeaderSearch` (pages publish their own search state), Donate button + `DonateModal`, `HeaderLanguage`
- runtime i18n `translate()` wrapping of page titles and descriptions
- Media Providers accordion, Skills, Translator, 9Remote, 9English
- provider detail breadcrumbs in the Header title (Providers phase)

Wiring notes from the first shell integration (`web/src/App.svelte`):

- The invented "Overview" area is gone; `/dashboard` resolves to Endpoint & Key, like upstream's index.
- Upstream has a dedicated `/login` page. Blackhand still shows the previous in-app login overlay,
  because the login page sits outside the seven in-scope areas; revisit when the auth surface is ported.
- The area bodies are still the previous implementations (legacy CSS classes) rendered inside the new
  shell, so their look is interim until each area is replaced by its 1:1 port.

## Endpoint & Key (phase 2, in progress)

Ported from the official checkout under `web/src/pages/endpoint/` and the shared primitives under
`web/src/ui/` (Button, Card, CardSection, CardRow, CardListItem, Input, Toggle, Modal, ConfirmModal,
Tooltip — all copied from `src/shared/components/` at the pin). Verified with `npm run check`
(0 errors, 0 warnings), `npm run build`, and the full `scripts/test-windows.ps1` run; the rendered
page was checked in a browser against the upstream structure.

Behaviour carried over: local endpoint row with copy feedback, `requireApiKey` toggle, API-key list
with masked/reveal/copy, create/delete/pause-confirm flows, per-key access restriction with an allow
list, and auto-provisioning of a "Default Key" for first-time users.

Still open in this area, so they are absent rather than faked:

- the Tunnel (Cloudflare) and Tailscale rows on the same upstream card, with their modals, ping
  loops and `/api/tunnel/*` routes, plus the "Allow dashboard access via tunnel" toggle
- `KeyAccessControls`' picker: upstream opens the shared `ModelSelectModal` fed by `/api/providers`
  and `/api/models/alias` (Providers area). Until those exist, the page opens a models-only picker
  fed by `/v1/models`; this is a deliberate interim, recorded here.

## Setting (profile) — phase 2, in progress

Ported from the official checkout into `web/src/pages/profile/ProfilePage.svelte`. Cards in place,
matching upstream markup and labels:

- **Local Mode** — theme segmented control (light / dark / system) driving the shell theme store, and
  the Database Location row. Upstream prints a fixed path; Blackhand prints the real one from
  `DATA_DIR`, which `GET /api/settings` now returns as `databasePath` (a small port adaptation).
- **Security** — "Require login" toggle (`PATCH /api/settings`), and the password form (current /
  new / confirm, with the current field shown only when a password is stored). Confirmation and
  error states driven by the server response.

Still open in this area, absent rather than faked:

- **Download Backup / Import Backup** — upstream exchanges a JSON payload through
  `/api/settings/database` (with a password re-auth header). Blackhand has no equivalent export
  format yet; a raw SQLite copy would not be compatible with upstream's import, so nothing is shown.
- **Language** — upstream opens `LanguageSwitcher` over the runtime i18n catalogue; Blackhand has no
  i18n layer yet.
- **Single Sign-On (SSO)** — OIDC and SAML configuration forms plus their test endpoints. The Go side
  stores the settings and auth already refuses to use unimplemented SSO flows.
- **Routing Strategy** (round robin, sticky limits) — belongs with provider selection.
- **Network** — outbound proxy fields exist in the Go settings; the upstream card and
  `/api/settings/proxy-test` are not ported.
- **Observability** and the shutdown action (`/api/version/shutdown`).

## Providers (phase 3, in progress)

Ported from the official checkout into `web/src/pages/providers/`:

- **Free Tier list** (`/dashboard/providers`) — the upstream "Free Tier Providers" heading and the
  provider card grid, restricted to the one provider in scope. The card carries the provider colour,
  the `Ready` badge for a no-auth provider, and links to the detail page.
- **Detail header** (`/dashboard/providers/opencode`) — back link, provider icon, name and the
  connection count, laid out like upstream.
- **NoAuthProxyCard** — the "No authentication required" card with the Proxy Pool selector and the
  Rotation Strategy selector. The per-provider override is stored exactly as upstream does it, in
  `settings.providerStrategies[providerId] = { proxyPoolId, rotateStrategy }` through
  `PATCH /api/settings`, with the same rotation hints and the "needs at least 2 active pools" rule.

New Go surface: `GET /api/providers` (`internal/httpapi/providers.go`) returns every stored
connection with its credential fields removed, mirroring upstream's `toProviderConnectionResponse`.
The `providerConnections` table already existed in the schema.

Also carried over: the provider icon asset `public/providers/opencode.png` from the reference
checkout, and the registry subset in `web/src/lib/providers.ts` (id, name, colour, text icon,
`noAuth`, category `freeTier`).

- **Custom Headers** — the collapsible card with add/remove rows, Reset and Save. Overrides live in
  `settings.providerOverrides[providerId].headers` exactly as upstream stores them, diffed against the
  registry's built-in transport headers (`x-opencode-client: desktop` for this provider, mirrored from
  `open-sse/providers/registry/opencode.js`). Validation is server side in
  `internal/httpapi/provider_overrides.go`: name charset, no CRLF, 20-header cap, 8 KiB value cap, and
  the blocked set (host, content-length, content-type, connection, transfer-encoding, authorization,
  cookie), with table tests in `provider_overrides_test.go`. Verified through the browser: saving an
  override persists it, and Reset + Save clears it again.
- **Available Models** — the catalogue read behind the dashboard session (`GET /api/models`, because the
  public `/v1/models` needs a client API key), one tile per model with copy and a disable action that
  moves the model to a dashed restore chip, plus "Disable All" with upstream's confirm text. Disabled
  state is kept per provider alias through `GET/POST/DELETE /api/models/disabled`, stored in the `kv`
  table (`internal/storage/disabled_models.go`, table tests in `disabled_models_test.go`). Verified
  through the browser: disabling `big-pickle` persisted `{"ids":["big-pickle"]}` and restoring it
  cleared the set again.

The card lists only the provider's free models, using the same rule upstream applies in
`api/providers/suggested-models/filters.js` (`opencode-free`): an id ending in `-free` or in the known
list (`big-pickle`), minus the ids upstream treats as dead (`deepseek-v4-flash-free`). The per-model
Test action posts to `/api/models/test`, which pings the model through this router's own inference
pipeline (a minimal non-streaming completion over loopback, carrying an internal unrestricted key) and
returns `{ok, error}` exactly like upstream's ping; the tile turns red/green and the error text is
shown under the heading.

Also fixed here: the area content now remounts when the dashboard session changes, because signing in
through the overlay used to leave a page holding the data it had fetched while unauthenticated.

Deviations recorded rather than hidden: the FREE badge, per-model capability badges and the thinking
suffix are absent because the catalogue this port fetches carries no `isFree`, caps or thinking
metadata; and `/api/models` does not filter the disabled set out of its response, because the card
needs the full list to offer restoring a model, where upstream's route filters it for its other
consumers.

Free-tier fingerprint, implemented from the upstream pull requests that address this exact refusal
(`fix(opencode): refresh the client fingerprint to the 2.0 channel form` #4883 and
`fix(opencode-zen): align free-tier v2 headers` #4860, both still open on the reference repo):

- `User-Agent: opencode/latest/2.0.29/cli` and `x-opencode-client: cli` (the v1 forms are refused),
- `x-opencode-project: global` and the v2 session set — `x-opencode-session` plus
  `x-opencode-session-id`, `x-session-affinity`, `x-session-id` carrying the same `ses_…` id,
- `Authorization: Bearer public` for the free tier unless an upstream key is configured,
- free models (`-free` base id, `isOpencodeFreeTierModel` in upstream terms) routed to
  `https://opencode.ai/inference/v1/chat/completions` instead of the `/zen/v1` lane, with the request
  forced to `stream: true` and OpenCode's own `read`/`shell` tool definitions appended verbatim, plus
  the placeholder note that tells the model not to call them.

All of that is pinned by tests in `internal/provider/opencode/fingerprint_test.go` and
`fingerprint_proxy_test.go`, which assert the exact path, headers and body this port puts on the wire.

Live result after the first attempt, and the correction that followed: with those headers alone the upstream
still answered `403 FreeTierError` ("OpenCode's free tier can only be used from within OpenCode"), even
though a captured request showed the exact path, headers and body the PRs prescribe — a hand-built
`curl` with the same bytes was refused identically. The missing piece was the **session id format**:
the 12 hex characters are not free-form. Upstream decodes them as the low 48 bits of
`~(timeMs*0x1000 + counter)` and refuses a counter outside a small range, so a random prefix is always
rejected. The working implementation lives in `Yoganataa/9router` `dev` (v0.5.111) — commits
"use canonical inverted-timestamp session IDs to prevent 403 FreeTierError", "restore valid bearer auth
syntax in session patch" and "restore free-tier inference compatibility" — which is what this port now
mirrors: `ses_` + inverted-timestamp prefix with a per-millisecond counter + 14 base62 characters.

Verified live afterwards: the model Test action answers `{"ok":true}` for `oc/mimo-v2.6-flash-free` and
`oc/deepseek-v4.1-flash-free`, and a real `POST /v1/chat/completions` streams the model's own output
back through the router (`HTTP 200`, `mimo-v2.6-flash-free`, reasoning deltas `Ready.`). Free-tier
inference works from this machine now; the earlier "not available in your country" reply seen in a
previous session's OpenCode log did not reappear.

Still open in this area, absent rather than faked:

- **Model extras** — thinking selector (`settings.providerThinking` plus
  `open-sse/providers/thinkingLevels.js`), "Suggested free models" as separate add-chips, and "Add
  Model" with custom models and aliases (`/api/models/custom`, `/api/models/alias`).
- **Connections** — list, add, edit, delete, bulk import, per-connection proxy pool binding and
  cooldown display (`/api/providers` POST/PUT/DELETE already have an upstream contract to match).
- **Batch actions** — "Test All" and `/api/providers/test-batch`.

## Rules

1. Official `master` is the source of truth; when this repo and upstream disagree, upstream wins.
2. Compare the response contract of every route before replacing a Blackhand placeholder.
3. No invented menu, statistic or button: anything not implemented is absent or explicitly recorded
   as a gap in this file.
4. Keep UI structure, labels, ordering, icons and layout from upstream; only the branding changes to
   Blackhand.
5. Re-derive every fixture from the pinned official checkout and record the sha next to it.
