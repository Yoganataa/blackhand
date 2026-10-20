package dashboard

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"9router/proxy/internal/tunnel/cloudflared"
)

// fakeCloudflared writes an executable that prints the given lines and then
// sleeps, so a supervisor can be driven without a real cloudflared binary.
// A .bat on Windows, a shell script elsewhere.
func fakeCloudflared(t *testing.T, dir string, lines []string) string {
	t.Helper()
	var path, body string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "cloudflared.bat")
		var b strings.Builder
		b.WriteString("@echo off\n")
		for _, line := range lines {
			b.WriteString("echo " + line + "\n")
		}
		b.WriteString("ping -n 31 127.0.0.1 >nul\n")
		body = b.String()
	} else {
		path = filepath.Join(dir, "cloudflared.sh")
		var b strings.Builder
		b.WriteString("#!/bin/sh\n")
		for _, line := range lines {
			b.WriteString("echo '" + line + "'\n")
		}
		b.WriteString("sleep 30\n")
		body = b.String()
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// useSupervisor installs a supervisor built from a fake cloudflared for the
// duration of the test. The supervisor is a process-wide singleton, so the
// test has to put it back afterwards or the next test inherits it.
func useSupervisor(t *testing.T, cfg cloudflared.Config) *cloudflared.Supervisor {
	t.Helper()
	sup := cloudflared.New(cfg)
	restore := cloudflared.SetShared(sup)
	t.Cleanup(restore)
	return sup
}

type tunnelStatusBody struct {
	Tunnel struct {
		Enabled    bool   `json:"enabled"`
		Running    bool   `json:"running"`
		TunnelURL  string `json:"tunnelUrl"`
		Mode       string `json:"mode"`
		Configured bool   `json:"configured"`
	} `json:"tunnel"`
}

func readTunnelStatus(t *testing.T, h *DashboardHandler) tunnelStatusBody {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/tunnel/status", nil)
	rec := httptest.NewRecorder()
	h.HandleTunnelStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var res tunnelStatusBody
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res
}

func TestTunnelStatusReportsSupervisorState(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("UnconfiguredModeIsOff", func(t *testing.T) {
		useSupervisor(t, cloudflared.Config{Mode: cloudflared.ModeOff})
		h := NewDashboardHandler(repo)

		res := readTunnelStatus(t, h)
		if res.Tunnel.Mode != cloudflared.ModeOff {
			t.Fatalf("mode = %q, want off", res.Tunnel.Mode)
		}
		if res.Tunnel.Configured {
			t.Fatal("configured must be false when the mode is off")
		}
	})

	t.Run("NamedModeIsConfigured", func(t *testing.T) {
		useSupervisor(t, cloudflared.Config{Mode: cloudflared.ModeNamed})
		h := NewDashboardHandler(repo)

		res := readTunnelStatus(t, h)
		if res.Tunnel.Mode != cloudflared.ModeNamed {
			t.Fatalf("mode = %q, want named", res.Tunnel.Mode)
		}
		if !res.Tunnel.Configured {
			t.Fatal("configured must be true when a tunnel mode is set")
		}
		// The settings row claims a tunnel, but no process is running: the
		// supervisor is the source of truth, so the status must say stopped.
		if err := repo.UpdateSettingsRaw(map[string]any{
			"tunnelEnabled": true,
			"tunnelUrl":     "https://stale.example.com",
		}); err != nil {
			t.Fatal(err)
		}
		res = readTunnelStatus(t, h)
		if res.Tunnel.Running {
			t.Fatal("running must follow the live process, not the settings row")
		}
		_ = repo.UpdateSettingsRaw(map[string]any{"tunnelEnabled": false, "tunnelUrl": ""})
	})
}

func TestTunnelEnableDisabledWhenNotConfigured(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	useSupervisor(t, cloudflared.Config{Mode: cloudflared.ModeOff})
	h := NewDashboardHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/tunnel/enable", nil)
	rec := httptest.NewRecorder()
	h.HandleTunnelEnable(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var res struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(res.Error, "CF_TUNNEL_MODE") {
		t.Fatalf("error should name the env var to set, got %q", res.Error)
	}
}

func TestCloudflareTunnelEnableAndDisable(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	dir := t.TempDir()
	bin := fakeCloudflared(t, dir, []string{"Registered tunnel connection"})
	sup := useSupervisor(t, cloudflared.Config{
		Mode:       cloudflared.ModeNamed,
		Bin:        bin,
		ConfigYAML: "ingress:\n  - hostname: blackhand.example.com\n    service: http://localhost:20130\n",
		WorkDir:    dir,
	})
	h := NewDashboardHandler(repo)

	enableReq := httptest.NewRequest(http.MethodPost, "/api/tunnel/cloudflare-enable", nil)
	enableRec := httptest.NewRecorder()
	h.HandleCloudflareTunnelEnable(enableRec, enableReq)

	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200: %s", enableRec.Code, enableRec.Body.String())
	}
	var enabled struct {
		Success   bool   `json:"success"`
		TunnelURL string `json:"tunnelUrl"`
		Running   bool   `json:"running"`
	}
	if err := json.Unmarshal(enableRec.Body.Bytes(), &enabled); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !enabled.Success || !enabled.Running {
		t.Fatalf("enable response = %+v", enabled)
	}
	if enabled.TunnelURL != "https://blackhand.example.com" {
		t.Fatalf("tunnelUrl = %q", enabled.TunnelURL)
	}

	// The status endpoint must agree with the enable response.
	res := readTunnelStatus(t, h)
	if !res.Tunnel.Running {
		t.Fatal("status must report the tunnel as running")
	}
	if res.Tunnel.TunnelURL != "https://blackhand.example.com" {
		t.Fatalf("status tunnelUrl = %q", res.Tunnel.TunnelURL)
	}

	// Disable must stop the process, not just clear the settings row.
	disableReq := httptest.NewRequest(http.MethodPost, "/api/tunnel/cloudflare-disable", nil)
	disableRec := httptest.NewRecorder()
	h.HandleCloudflareTunnelDisable(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable status = %d", disableRec.Code)
	}
	if sup.Status().Running {
		t.Fatal("disable must stop the supervised process")
	}
	res = readTunnelStatus(t, h)
	if res.Tunnel.Running {
		t.Fatal("status must report the tunnel as stopped after disable")
	}
}

func TestTunnelDisableStopsSupervisedProcess(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	dir := t.TempDir()
	bin := fakeCloudflared(t, dir, []string{"Registered tunnel connection"})
	sup := useSupervisor(t, cloudflared.Config{
		Mode:       cloudflared.ModeNamed,
		Bin:        bin,
		ConfigYAML: "ingress:\n  - hostname: blackhand.example.com\n    service: http://localhost:20130\n",
		WorkDir:    dir,
	})
	h := NewDashboardHandler(repo)

	enableReq := httptest.NewRequest(http.MethodPost, "/api/tunnel/cloudflare-enable", nil)
	enableRec := httptest.NewRecorder()
	h.HandleCloudflareTunnelEnable(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d", enableRec.Code)
	}
	if !sup.Status().Running {
		t.Fatal("supervisor should be running after enable")
	}

	// The legacy disable route must stop the process too, otherwise the
	// tunnel keeps serving traffic with no trace in the settings.
	disableReq := httptest.NewRequest(http.MethodPost, "/api/tunnel/disable", nil)
	disableRec := httptest.NewRecorder()
	h.HandleTunnelDisable(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable status = %d", disableRec.Code)
	}
	if sup.Status().Running {
		t.Fatal("disable must stop the supervised process")
	}
}

// TestSupervisorIsSharedAcrossHandlers pins that the tunnel supervisor is a
// process-wide singleton. The gateway builds several DashboardHandler values
// (one per route table), and a per-handler supervisor would let the status
// endpoint and the enable endpoint disagree about the same tunnel.
func TestSupervisorIsSharedAcrossHandlers(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	useSupervisor(t, cloudflared.Config{Mode: cloudflared.ModeNamed})
	first := NewDashboardHandler(repo)
	second := NewDashboardHandler(repo)

	if first.tunnelSupervisor() != second.tunnelSupervisor() {
		t.Fatal("two handlers must share one supervisor")
	}
}
