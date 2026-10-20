#!/usr/bin/env python3
"""Colab bootstrap for Blackhand.

Starts the Blackhand gateway on Google Colab, wired for a named Cloudflare
Tunnel.

Colab has no Go toolchain and no Bun, and the binary embeds the built web
app, so the runtime cannot compile the gateway itself. The bootstrap
therefore resolves the binary in this order:

  1. BLACKHAND_BINARY - a path to a binary the operator already placed there.
  2. A cached build from a previous run (kept outside the repo, so a
     re-clone does not lose it).
  3. A prebuilt release binary for linux/amd64, downloaded from
     BLACKHAND_BINARY_URL or from the fork's release page.
  4. A source build, which installs Go and Bun first. Slow, but it needs no
     release to exist.

Steps 1-3 are the fast paths and are what a notebook should use; step 4 is
the fallback for a fork that has not cut a release yet.

Usage in a Colab notebook cell:

    !git clone https://github.com/Yoganataa/9router-go.git
    %cd 9router-go
    !python3 scripts/colab-bootstrap.py

Or with the environment set beforehand:

    import os
    os.environ["JWT_SECRET"] = "..."
    os.environ["INITIAL_PASSWORD"] = "..."
    os.environ["ROUTER_API_KEY"] = "sk-..."
    os.environ["CF_TUNNEL_MODE"] = "named"
    os.environ["CF_TUNNEL_CONFIG_FILE"] = "/content/config.yml"
    os.environ["CF_TUNNEL_CREDENTIALS_FILE"] = "/content/credentials.json"
    !python3 scripts/colab-bootstrap.py

The script is idempotent: re-running it stops the previous process first.
"""

import json
import os
import shutil
import signal
import stat
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

PORT = int(os.environ.get("PORT", "20130"))
DATA_DIR = os.environ.get("DATA_DIR", "/content/blackhand-data")
CACHE_DIR = Path(os.environ.get("BLACKHAND_CACHE", "/content/blackhand-cache"))
REPO_DIR = Path(os.environ.get("BLACKHAND_REPO", ".")).resolve()
PID_FILE = CACHE_DIR / "blackhand.pid"
LOG_DIR = Path(DATA_DIR)

# The release page of the fork. Overridable, because a fork that has not cut
# a release yet has no asset to download.
DEFAULT_BINARY_URL = os.environ.get(
    "BLACKHAND_BINARY_URL",
    "https://github.com/Yoganataa/9router-go/releases/latest/download/blackhand-linux-amd64",
)
GO_VERSION = os.environ.get("BLACKHAND_GO_VERSION", "1.27.0")
BUN_VERSION = os.environ.get("BLACKHAND_BUN_VERSION", "1.4.2")


def log(message: str) -> None:
    print(f"[bootstrap] {message}", flush=True)


def run(command, cwd=None, env=None, check=True) -> subprocess.CompletedProcess:
    return subprocess.run(
        command,
        cwd=cwd,
        env=env,
        check=check,
        capture_output=True,
        text=True,
    )


def download(url: str, target: Path, timeout: int = 300) -> bool:
    """Fetch url into target. Returns False instead of raising, so the caller
    can fall through to the next strategy."""
    try:
        log(f"downloading {url}")
        request = urllib.request.Request(url, headers={"User-Agent": "blackhand-colab"})
        with urllib.request.urlopen(request, timeout=timeout) as response:
            target.write_bytes(response.read())
    except (urllib.error.URLError, urllib.error.HTTPError, OSError, TimeoutError) as error:
        log(f"download failed: {error}")
        return False
    target.chmod(target.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return True


def process_alive(pid: int) -> bool:
    """Report whether a process is still running.

    os.kill(pid, 0) is the POSIX probe, but on Windows it calls
    TerminateProcess with exit code 0 - it would kill the process it is
    meant to inspect, and it can land on a recycled pid. Tasklist is the
    Windows equivalent.
    """
    if os.name == "nt":
        result = subprocess.run(
            ["tasklist", "/FI", f"PID eq {pid}", "/NH"],
            capture_output=True,
            text=True,
        )
        return str(pid) in result.stdout
    try:
        os.kill(pid, 0)
        return True
    except (ProcessLookupError, PermissionError):
        return False


def go_version() -> str:
    """The Go version the repository needs, taken from its go.mod."""
    if os.environ.get("BLACKHAND_GO_VERSION"):
        return os.environ["BLACKHAND_GO_VERSION"]
    gomod = REPO_DIR / "go.mod"
    if gomod.exists():
        for line in gomod.read_text().splitlines():
            if line.startswith("go "):
                version = line.split()[1].strip()
                # go.mod carries a language version ("1.27"), not a patch
                # release. The toolchain download needs one.
                if version.count(".") == 1:
                    version = f"{version}.0"
                return version
    return GO_VERSION


def ensure_cloudflared() -> None:
    """Put a cloudflared binary on PATH, downloading one if the runtime has none.

    The tunnel supervisor runs the real cloudflared, and Colab does not ship
    it. Without this the tunnel cannot start at all, and the failure looks
    like a configuration problem rather than a missing binary.
    """
    if os.environ.get("CF_TUNNEL_MODE", "off") == "off":
        return
    if os.environ.get("CF_TUNNEL_BIN"):
        return
    existing = shutil.which("cloudflared")
    if existing:
        os.environ["CF_TUNNEL_BIN"] = existing
        return

    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    target = CACHE_DIR / "cloudflared"
    url = os.environ.get(
        "BLACKHAND_CLOUDFLARED_URL",
        "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64",
    )
    if not download(url, target):
        sys.exit("[bootstrap] ERROR: could not download cloudflared; set CF_TUNNEL_BIN to a binary you placed there")
    os.environ["CF_TUNNEL_BIN"] = str(target)
    log(f"installed cloudflared at {target}")


def binary_path() -> Path:
    """The path the gateway binary is expected at, once resolved."""
    override = os.environ.get("BLACKHAND_BINARY")
    if override:
        return Path(override)
    return REPO_DIR / "blackhand"


def resolve_binary() -> Path:
    """Find a runnable gateway binary, building one only if nothing else works."""
    explicit = os.environ.get("BLACKHAND_BINARY")
    if explicit:
        path = Path(explicit)
        if not path.exists():
            sys.exit(f"[bootstrap] ERROR: BLACKHAND_BINARY={explicit} does not exist")
        return path

    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    cached = CACHE_DIR / "blackhand"
    if cached.exists() and os.access(cached, os.X_OK):
        log(f"using the cached binary at {cached}")
        return cached

    target = binary_path()
    if target.exists() and os.access(target, os.X_OK):
        log(f"using the binary at {target}")
        return target

    if download(DEFAULT_BINARY_URL, cached, timeout=30):
        log(f"using the downloaded release binary at {cached}")
        return cached

    log("no prebuilt binary available; building from source (this takes a few minutes)")
    return build_from_source(cached)


def install_go() -> Path:
    """Install the Go toolchain next to the cache and return its bin directory."""
    toolchain = CACHE_DIR / "go"
    if (toolchain / "bin" / "go").exists():
        log("Go is already installed")
        return toolchain / "bin"

    version = go_version()
    url = f"https://go.dev/dl/go{version}.linux-amd64.tar.gz"
    archive = CACHE_DIR / "go.tar.gz"
    if not download(url, archive):
        sys.exit(f"[bootstrap] ERROR: could not download Go {version}; set BLACKHAND_GO_VERSION to a released version")
    run(["tar", "-C", str(CACHE_DIR), "-xzf", str(archive)])
    log(f"installed Go {version}")
    return toolchain / "bin"


def install_bun() -> Path:
    """Install Bun and return its directory, which holds the bun executable."""
    bun = shutil.which("bun")
    if bun:
        return Path(bun).parent
    target = CACHE_DIR / "bun"
    if (target / "bun").exists():
        return target
    target.mkdir(parents=True, exist_ok=True)
    script = CACHE_DIR / "install-bun.sh"
    if not download("https://bun.sh/install", script):
        sys.exit("[bootstrap] ERROR: could not download the Bun installer")
    script.chmod(script.stat().st_mode | stat.S_IXUSR)
    # The installer takes the version as an argument; the URL does not carry
    # it, so a pinned version has to be passed in.
    result = run(["bash", str(script), f"bun-v{BUN_VERSION}"], check=False)
    if result.returncode != 0:
        sys.exit(f"[bootstrap] ERROR: Bun install failed: {result.stderr[-500:]}")
    installed = Path(os.path.expanduser("~/.bun/bin/bun"))
    if not installed.exists():
        sys.exit("[bootstrap] ERROR: Bun install reported success but no binary was found")
    return installed.parent


def build_from_source(cached: Path) -> Path:
    """Build the gateway from the checked-out source, web app included."""
    if not (REPO_DIR / "go.mod").exists():
        sys.exit(f"[bootstrap] ERROR: no go.mod in {REPO_DIR}; nothing to build")

    go_bin = install_go()
    bun_bin = install_bun()
    env = dict(os.environ)
    env["PATH"] = f"{go_bin}{os.pathsep}{bun_bin}{os.pathsep}{env.get('PATH', '')}"
    env["CGO_ENABLED"] = "0"

    log("building the web app")
    result = run(["bun", "install", "--frozen-lockfile"], cwd=REPO_DIR, env=env, check=False)
    if result.returncode != 0:
        # A lockfile mismatch should not block a source build; the app is
        # embedded, but a slightly different dependency set is better than no
        # gateway at all.
        log("bun install --frozen-lockfile failed; retrying without the lockfile")
        run(["bun", "install"], cwd=REPO_DIR, env=env, check=False)
    run(["bun", "run", "build"], cwd=REPO_DIR / "web", env=env)

    log("building the gateway")
    run(
        ["go", "build", "-trimpath", "-ldflags", "-s -w", "-o", str(cached), "./cmd/9router-go/"],
        cwd=REPO_DIR,
        env=env,
    )
    log(f"built {cached}")
    return cached


def materialize_tunnel_files() -> None:
    """Write inline tunnel config and credentials to disk when the environment
    carries them instead of a path."""
    work = Path(DATA_DIR) / "cloudflared"
    work.mkdir(parents=True, exist_ok=True)
    if os.environ.get("CF_TUNNEL_CONFIG") and not os.environ.get("CF_TUNNEL_CONFIG_FILE"):
        target = work / "config.yml"
        target.write_text(os.environ["CF_TUNNEL_CONFIG"])
        os.environ["CF_TUNNEL_CONFIG_FILE"] = str(target)
    if os.environ.get("CF_TUNNEL_CREDENTIALS") and not os.environ.get("CF_TUNNEL_CREDENTIALS_FILE"):
        target = work / "credentials.json"
        target.write_text(os.environ["CF_TUNNEL_CREDENTIALS"])
        os.environ["CF_TUNNEL_CREDENTIALS_FILE"] = str(target)


def stop_previous() -> None:
    if not PID_FILE.exists():
        return
    try:
        pid = int(PID_FILE.read_text().strip())
    except ValueError:
        PID_FILE.unlink(missing_ok=True)
        return
    try:
        os.kill(pid, signal.SIGTERM)
    except (ProcessLookupError, PermissionError, OSError):
        PID_FILE.unlink(missing_ok=True)
        return
    # Give it a moment to shut down, then insist.
    for _ in range(40):
        time.sleep(0.25)
        if not process_alive(pid):
            break
    else:
        try:
            os.kill(pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError, OSError):
            pass
    PID_FILE.unlink(missing_ok=True)


def wait_ready(timeout: int = 60) -> bool:
    url = f"http://127.0.0.1:{PORT}/health"
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=2) as response:
                if response.status == 200:
                    return True
        except Exception:
            time.sleep(0.5)
    return False


def dashboard_login(password: str) -> str:
    """Log in to the dashboard and return the session cookie."""
    payload = json.dumps({"password": password}).encode()
    request = urllib.request.Request(
        f"http://127.0.0.1:{PORT}/api/auth/login",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        cookie = response.headers.get("Set-Cookie", "")
    return cookie.split(";")[0]


def enable_tunnel(cookie: str) -> str:
    request = urllib.request.Request(
        f"http://127.0.0.1:{PORT}/api/tunnel/cloudflare-enable",
        data=b"{}",
        headers={"Content-Type": "application/json", "Cookie": cookie},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=60) as response:
        body = json.loads(response.read())
    return body.get("tunnelUrl", "")


def write_status(tunnel_url: str) -> None:
    """Record what the runtime ended up with, so a notebook can read the
    result without parsing the log."""
    status = {
        "tunnelUrl": tunnel_url,
        "endpoint": (tunnel_url.rstrip("/") + "/v1") if tunnel_url else "",
        "port": PORT,
        "dataDir": DATA_DIR,
        "pidFile": str(PID_FILE),
    }
    (LOG_DIR / "status.json").write_text(json.dumps(status, indent=2))
    log(f"wrote {LOG_DIR / 'status.json'}")


def main() -> None:
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    CACHE_DIR.mkdir(parents=True, exist_ok=True)

    os.environ.setdefault("DATA_DIR", DATA_DIR)
    os.environ.setdefault("PORT", str(PORT))
    os.environ.setdefault("JWT_SECRET", "colab-ephemeral-secret-change-me")
    os.environ.setdefault("INITIAL_PASSWORD", "colab")
    os.environ.setdefault("AUTO_UPDATE", "false")
    os.environ.setdefault("CF_TUNNEL_MODE", "off")

    # The tunnel gate refuses the dashboard on a tunnel hostname unless
    # dashboard access is enabled there, and on Colab the tunnel is the only
    # way in. The dashboard still needs the password: requireLogin defaults to
    # on, and this setting only lifts the hostname gate.
    if os.environ.get("CF_TUNNEL_MODE", "off") != "off":
        os.environ.setdefault("ROUTER_TUNNEL_DASHBOARD_ACCESS", "true")
        log("ROUTER_TUNNEL_DASHBOARD_ACCESS defaults to true: the tunnel is the only way into this runtime")

    stop_previous()
    materialize_tunnel_files()
    ensure_cloudflared()
    binary = resolve_binary()

    stdout = open(LOG_DIR / "stdout.log", "w")
    stderr = open(LOG_DIR / "stderr.log", "w")
    process = subprocess.Popen(
        [str(binary), "--auto-update=false"],
        env=dict(os.environ),
        stdin=subprocess.DEVNULL,
        stdout=stdout,
        stderr=stderr,
        start_new_session=True,  # survive the notebook cell that started it
    )
    PID_FILE.write_text(str(process.pid))
    log(f"started {binary} (pid {process.pid}) on port {PORT}")

    if not wait_ready():
        log("ERROR: the gateway did not become ready")
        tail = (LOG_DIR / "stderr.log").read_text()[-2000:]
        log(tail)
        sys.exit(1)
    log(f"the gateway is ready on http://127.0.0.1:{PORT}")

    if os.environ.get("CF_TUNNEL_MODE", "off") != "off":
        try:
            cookie = dashboard_login(os.environ.get("INITIAL_PASSWORD", "colab"))
            url = enable_tunnel(cookie)
            log(f"tunnel URL: {url}")
            write_status(url)
        except Exception as error:
            log(f"could not start the tunnel: {error}")
            log((LOG_DIR / "stderr.log").read_text()[-1000:])

    log(f"dashboard: log in with INITIAL_PASSWORD={os.environ.get('INITIAL_PASSWORD')}")
    log(f"logs: {LOG_DIR / 'stdout.log'} and {LOG_DIR / 'stderr.log'}")


if __name__ == "__main__":
    main()
