# Windows verification

This guide targets PowerShell 7 on Windows. The checks use a temporary data directory and never open your active 9Router database.

## 1. Update the branch

From the existing blackhand clone:

    git switch work/go-port-foundation
    git pull --ff-only
    git log -1 --oneline

## 2. Run the automated Windows checks

From the repository root:

    pwsh -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-windows.ps1

The script checks Go tests and go vet, installs/checks/builds the Svelte frontend, embeds it, builds bin\blackhand.exe, starts that executable with a brand-new temporary DATA_DIR, checks /readyz and the homepage, then stops the process and removes the temporary data directory. It does not inspect or modify a real database.

Expected final message:

    PASS: Go tests, vet, frontend checks, Windows build, /readyz and homepage.

Do not treat a build as successful merely because a script prints a line after a failed command; the script checks every native command's exit code and terminates on failure. Svelte accessibility/type-check warnings should be fixed rather than ignored; a warning-free npm run check is the goal.

## 3. Start a clean local instance manually

After the script succeeds, open PowerShell at the repository root:

    $env:HOST = "127.0.0.1"
    $env:PORT = "20128"
    $env:DATA_DIR = Join-Path $PWD "test-clean-data"
    $env:INITIAL_PASSWORD = "Set-A-Unique-Local-Test-Password"
    $env:NINEROUTER_API_KEY = ""
    $env:REQUIRE_API_KEY = "true"
    .\bin\blackhand.exe

Open http://127.0.0.1:20128/. Keep the terminal open while testing. Use only the local test password and newly created test API keys. In a second PowerShell window verify:

    curl.exe -i http://127.0.0.1:20128/healthz
    curl.exe -i http://127.0.0.1:20128/readyz
    curl.exe -i http://127.0.0.1:20128/
    curl.exe -i http://127.0.0.1:20128/v1/models

Health, readiness and homepage should return 200. Without an API key, /v1/models should return 401 when API-key enforcement is enabled. Stop the application with Ctrl+C.

## 4. Test database migration only on a snapshot

Do not point Blackhand at the live 9Router data directory. Stop 9Router cleanly first and use SQLite's backup API (not a plain copy of an active data.sqlite file) to create a snapshot. Check the snapshot with PRAGMA integrity_check and record baseline row counts for settings, apiKeys, usageHistory, usageDaily, proxyPools, providerConnections, combos, kv, and requestDetails.

Copy the snapshot into a new, disposable test directory named, for example, test-migration-data\db\data.sqlite, set DATA_DIR to that test directory, and start Blackhand. Do not use the original location.

After startup, stop Blackhand and verify:

- PRAGMA integrity_check remains ok.
- _meta.schemaVersion is 2 if migration started from version 1.
- A pre-change backup exists under db\backups\pre-schema-*.sqlite; it should still show the original schema version and baseline rows.
- Counts and values in settings, API keys, usage history/daily aggregates, proxy pools, provider connections and other populated tables match the snapshot baseline.
- The dashboard opens and an existing migrated API key authenticates only if it is active and the configured policy allows it.

If any values are missing or the backup cannot be opened, stop here. Keep the source database untouched and retain the disposable copy plus error output for diagnosis. Remove the test directory only after you no longer need it for troubleshooting.

## 5. What to report on failure

Copy the failing command, full error text, and the final CI link. For migration issues, send only integrity_check, schema version, table row counts, and redacted errors. Do not send data.sqlite, raw API keys, dashboard passwords, proxy URLs containing credentials, or other secrets.
