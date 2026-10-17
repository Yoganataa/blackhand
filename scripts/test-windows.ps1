$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Command,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    Write-Host ""
    Write-Host ("> " + $Command + " " + ($Arguments -join " ")) -ForegroundColor Cyan
    & $Command @Arguments
    $code = $LASTEXITCODE
    if ($code -ne 0) {
        throw ("Command failed with exit code " + $code + ": " + $Command + " " + ($Arguments -join " "))
    }
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$WebDir = Join-Path $RepoRoot "web"
$WebDist = Join-Path $WebDir "dist"
$AssetsDir = Join-Path $RepoRoot "internal\httpapi\assets"
$BinDir = Join-Path $RepoRoot "bin"
$ExePath = Join-Path $BinDir "blackhand.exe"

foreach ($tool in @("go", "node", "npm")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "Required command '$tool' was not found on PATH."
    }
}

Push-Location $RepoRoot
try {
    Write-Host "== Normalize Go module graph ==" -ForegroundColor Yellow
    Invoke-NativeCommand -Command "go" -Arguments @("mod", "tidy")
    $moduleDiff = & git diff -- go.mod go.sum
    if ($moduleDiff) {
        Write-Host "Go module files changed by go mod tidy (these changes should be committed):" -ForegroundColor DarkYellow
        Write-Host ($moduleDiff -join [Environment]::NewLine)
    }

    Write-Host "== Go tests ==" -ForegroundColor Yellow
    Invoke-NativeCommand -Command "go" -Arguments @("test", "./...")

    Write-Host "== Go vet ==" -ForegroundColor Yellow
    Invoke-NativeCommand -Command "go" -Arguments @("vet", "./...")

    Write-Host "== Frontend install, check and build ==" -ForegroundColor Yellow
    Push-Location $WebDir
    try {
        Invoke-NativeCommand -Command "npm" -Arguments @("install")
        Invoke-NativeCommand -Command "npm" -Arguments @("run", "check")
        Invoke-NativeCommand -Command "npm" -Arguments @("run", "build")
    }
    finally {
        Pop-Location
    }

    if (-not (Test-Path -LiteralPath $WebDist -PathType Container)) {
        throw "Frontend build did not create web/dist."
    }

    Write-Host "== Embed frontend and build Windows executable ==" -ForegroundColor Yellow
    New-Item -ItemType Directory -Force -Path $AssetsDir, $BinDir | Out-Null

    # Preserve the tracked placeholder and remove only previously generated assets.
    Get-ChildItem -LiteralPath $AssetsDir -Force |
        Where-Object { $_.Name -ne ".gitkeep" } |
        Remove-Item -Recurse -Force
    Get-ChildItem -LiteralPath $WebDist -Force |
        Copy-Item -Destination $AssetsDir -Recurse -Force

    Invoke-NativeCommand -Command "go" -Arguments @(
        "build", "-trimpath", "-ldflags=-s -w", "-o", $ExePath, "./cmd/blackhand"
    )

    if (-not (Test-Path -LiteralPath $ExePath -PathType Leaf)) {
        throw "Build returned success, but bin/blackhand.exe was not created."
    }

    Write-Host "== Smoke-test packaged executable ==" -ForegroundColor Yellow
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()

    $SmokeData = Join-Path ([System.IO.Path]::GetTempPath()) ("blackhand-smoke-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $SmokeData | Out-Null

    $stdoutTask = $null
    $stderrTask = $null
    $proc = [System.Diagnostics.Process]::new()
    $started = $false
    $failure = $null
    $stdout = ""
    $stderr = ""

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $ExePath
    $startInfo.WorkingDirectory = $RepoRoot
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.CreateNoWindow = $true
    $startInfo.Environment["HOST"] = "127.0.0.1"
    $startInfo.Environment["PORT"] = [string]$port
    $startInfo.Environment["DATA_DIR"] = $SmokeData
    $startInfo.Environment["INITIAL_PASSWORD"] = "Blackhand-Smoke-Test-Only-2026!"
    $startInfo.Environment["NINEROUTER_API_KEY"] = ""
    $startInfo.Environment["REQUIRE_API_KEY"] = "true"

    $proc.StartInfo = $startInfo
    try {
        $started = $proc.Start()
        if (-not $started) {
            throw "Windows could not start the packaged executable."
        }

        $stdoutTask = $proc.StandardOutput.ReadToEndAsync()
        $stderrTask = $proc.StandardError.ReadToEndAsync()

        $ready = $false
        for ($attempt = 0; $attempt -lt 40; $attempt++) {
            if ($proc.HasExited) { break }
            try {
                $health = Invoke-WebRequest -Uri ("http://127.0.0.1:" + $port + "/readyz") -TimeoutSec 2
                if ($health.StatusCode -eq 200) {
                    $ready = $true
                    break
                }
            }
            catch {
                # The server may need a moment to open its listener.
            }
            Start-Sleep -Milliseconds 250
        }

        if (-not $ready) {
            $failure = "Executable smoke test failed: /readyz did not return HTTP 200."
        }
        else {
            $home = Invoke-WebRequest -Uri ("http://127.0.0.1:" + $port + "/") -TimeoutSec 5
            if ($home.StatusCode -ne 200 -or $home.Content -notmatch "Blackhand") {
                $failure = "Executable smoke test failed: homepage was not served as expected."
            }
        }
    }
    catch {
        $failure = $_.Exception.Message
    }
    finally {
        if ($started -and -not $proc.HasExited) {
            try {
                $proc.Kill($true)
                $proc.WaitForExit(5000) | Out-Null
            }
            catch {
                Write-Warning ("Could not stop smoke-test process cleanly: " + $_.Exception.Message)
            }
        }

        if ($null -ne $stdoutTask) {
            try { $stdout = $stdoutTask.GetAwaiter().GetResult() } catch {}
        }
        if ($null -ne $stderrTask) {
            try { $stderr = $stderrTask.GetAwaiter().GetResult() } catch {}
        }

        $proc.Dispose()
        Remove-Item -LiteralPath $SmokeData -Recurse -Force -ErrorAction SilentlyContinue
    }

    if ($null -ne $failure) {
        $newLine = [Environment]::NewLine
        throw ($failure + $newLine + "--- executable stdout ---" + $newLine + $stdout + $newLine + "--- executable stderr ---" + $newLine + $stderr)
    }

    Write-Host ""
    Write-Host "PASS: Go tests, vet, frontend checks, Windows build, /readyz and homepage." -ForegroundColor Green
    Write-Host ("Executable: " + $ExePath)
    Write-Host "No production database was opened or modified."
}
finally {
    Pop-Location
}
