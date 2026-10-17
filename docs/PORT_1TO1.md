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
| 2 | Setting (`/dashboard/profile`) + Endpoint & Key (incl. tunnel/Tailscale) | Endpoint & Key page ported (keys, access controls, require-API-key, security gate); tunnel/Tailscale rows and the shared model picker still open. Setting not started. |
| 3 | Providers — OpenCode Free: list + detail, connections, model enable/disable/alias/custom, proxy pool binding, custom headers | not started |
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

## Rules

1. Official `master` is the source of truth; when this repo and upstream disagree, upstream wins.
2. Compare the response contract of every route before replacing a Blackhand placeholder.
3. No invented menu, statistic or button: anything not implemented is absent or explicitly recorded
   as a gap in this file.
4. Keep UI structure, labels, ordering, icons and layout from upstream; only the branding changes to
   Blackhand.
5. Re-derive every fixture from the pinned official checkout and record the sha next to it.
