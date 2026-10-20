#!/usr/bin/env python3
"""Colab bootstrap for Blackhand.

Starts the Blackhand gateway on Google Colab with:
  - Ephemeral DATA_DIR (reset on restart, reproducible via env vars)
  - Settings + API keys injected from environment variables (see README)
  - Optional named Cloudflare Tunnel (CF_TUNNEL_MODE=named + files/env)

Usage in a Colab notebook cell:

    !git clone https://github.com/Yoganataa/blackhand.git
    %cd blackhand
    !python3 scripts/colab-bootstrap.py

Or with env overrides:

    import os
    os.environ["JWT_SECRET"] = "..."
    os.environ["INITIAL_PASSWORD"] = "..."
    os.environ["ROUTER_API_KEY"] = "sk-..."
    os.environ["CF_TUNNEL_MODE"] = "named"
    os.environ["CF_TUNNEL_CONFIG_FILE"] = "/content/config.yml"
    os.environ["CF_TUNNEL_CREDENTIALS_FILE"] = "/content/creds.json"
    !python3 scripts/colab-bootstrap.py

The script is idempotent: re-running it kills the previous process and restarts.
"""

import json
import os
import signal
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

PORT = int(os.environ.get("PORT", "20130"))
DATA_DIR = os.environ.get("DATA_DIR", "/content/blackhand-data")
BINARY = os.environ.get("BLACKHAND_BINARY", "./blackhand")
PID_FILE = Path("/content/blackhand.pid")


def ensure_binary():
    p = Path(BINARY)
    if p.exists() and p.is_file():
        return p
    # Try building from source if Go is available.
    if Path("go.mod").exists():
        print("[bootstrap] building from source...")
        subprocess.run(["go", "build", "-o", BINARY, "./cmd/9router-go/"], check=True)
        return p
    print("[bootstrap] ERROR: binary not found and no go.mod; set BLACKHAND_BINARY")
    sys.exit(1)


def materialize_tunnel_files():
    """Write inline tunnel config/credentials to disk if provided via env."""
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


def kill_previous():
    if PID_FILE.exists():
        try:
            pid = int(PID_FILE.read_text().strip())
            os.kill(pid, signal.SIGTERM)
            time.sleep(1)
        except (ValueError, ProcessLookupError, OSError):
            pass
        PID_FILE.unlink(missing_ok=True)


def wait_ready(timeout=30):
    url = f"http://127.0.0.1:{PORT}/api/health"
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=2) as r:
                if r.status == 200:
                    return True
        except Exception:
            time.sleep(0.5)
    return False


def main():
    os.makedirs(DATA_DIR, exist_ok=True)
    os.environ.setdefault("DATA_DIR", DATA_DIR)
    os.environ.setdefault("PORT", str(PORT))
    os.environ.setdefault("JWT_SECRET", "colab-ephemeral-secret-change-me")
    os.environ.setdefault("INITIAL_PASSWORD", "colab")
    os.environ.setdefault("AUTO_UPDATE", "false")
    os.environ.setdefault("CF_TUNNEL_MODE", "off")

    kill_previous()
    materialize_tunnel_files()
    binary = ensure_binary()

    env = dict(os.environ)
    print(f"[bootstrap] starting {binary} on :{PORT} (DATA_DIR={DATA_DIR})")
    proc = subprocess.Popen(
        [str(binary), "--auto-update=false"],
        env=env,
        stdout=open(f"{DATA_DIR}/stdout.log", "w"),
        stderr=open(f"{DATA_DIR}/stderr.log", "w"),
    )
    PID_FILE.write_text(str(proc.pid))

    if not wait_ready():
        print("[bootstrap] ERROR: server did not become ready")
        print(Path(f"{DATA_DIR}/stderr.log").read_text()[-2000:])
        sys.exit(1)

    print(f"[bootstrap] server ready on http://127.0.0.1:{PORT}")

    # Enable the named/quick tunnel if configured.
    mode = os.environ.get("CF_TUNNEL_MODE", "off")
    if mode != "off":
        import urllib.request
        req = urllib.request.Request(
            f"http://127.0.0.1:{PORT}/api/tunnel/cloudflare-enable",
            data=b"{}",
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        # Need an auth cookie; for Colab we use the initial password login.
        login_data = json.dumps({"password": os.environ.get("INITIAL_PASSWORD", "colab")}).encode()
        login_req = urllib.request.Request(
            f"http://127.0.0.1:{PORT}/api/auth/login",
            data=login_data,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(login_req) as r:
                cookie = r.headers.get("Set-Cookie", "")
            req.add_header("Cookie", cookie.split(";")[0])
            with urllib.request.urlopen(req) as r:
                body = json.loads(r.read())
                print(f"[bootstrap] tunnel URL: {body.get('tunnelUrl')}")
        except Exception as e:
            print(f"[bootstrap] tunnel enable failed: {e}")
            print(Path(f"{DATA_DIR}/stderr.log").read_text()[-1000:])

    # Print a ready-to-use API key hint.
    print("[bootstrap] done. Use INITIAL_PASSWORD to log in to the dashboard.")


if __name__ == "__main__":
    main()