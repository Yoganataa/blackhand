package bootinject

import (
	"path/filepath"
	"testing"

	"9router/proxy/internal/db"
)

func newTestRepo(t *testing.T) *db.Repo {
	t.Helper()
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "data.sqlite"))
	if err != nil {
		t.Fatalf("OpenDatabase failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.EnsureCoreSchema(conn); err != nil {
		t.Fatalf("EnsureCoreSchema failed: %v", err)
	}
	return db.NewRepo(conn)
}

func TestApplySeedsApiKeyAndSettings(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	t.Setenv("ROUTER_API_KEY", "sk-injected-test-key")
	t.Setenv("ROUTER_API_KEY_NAME", "colab")
	t.Setenv("ROUTER_SETTINGS_JSON", `{"requireLogin":false,"requireApiKey":true,"rtkEnabled":false,"headroomUrl":"http://127.0.0.1:8787"}`)

	repo := newTestRepo(t)
	Apply(repo)

	settings, err := repo.GetSettingsRaw()
	if err != nil {
		t.Fatal(err)
	}
	if settings["requireLogin"] != false {
		t.Fatalf("requireLogin = %v", settings["requireLogin"])
	}
	if settings["requireApiKey"] != true {
		t.Fatalf("requireApiKey = %v", settings["requireApiKey"])
	}
	if settings["rtkEnabled"] != false {
		t.Fatalf("rtkEnabled = %v", settings["rtkEnabled"])
	}
	if settings["headroomUrl"] != "http://127.0.0.1:8787" {
		t.Fatalf("headroomUrl = %v", settings["headroomUrl"])
	}

	keys, err := repo.GetApiKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Key != "sk-injected-test-key" || keys[0].Name == nil || *keys[0].Name != "colab" {
		t.Fatalf("seeded key = %#v", keys)
	}
	if keys[0].IsActive != 1 {
		t.Fatal("seeded key is not active")
	}

	// Idempotent: a second Apply does not duplicate the key.
	Apply(repo)
	keys2, err := repo.GetApiKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys2) != 1 {
		t.Fatalf("Apply is not idempotent: %d keys after second run", len(keys2))
	}
}

func TestApplyEnvSugarVars(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	t.Setenv("ROUTER_RTK_ENABLED", "false")
	t.Setenv("ROUTER_CAVEMAN_ENABLED", "true")
	t.Setenv("ROUTER_CAVEMAN_LEVEL", "ultra")
	t.Setenv("ROUTER_HEADROOM_TIMEOUT_MS", "1500")

	repo := newTestRepo(t)
	Apply(repo)

	settings, err := repo.GetSettingsRaw()
	if err != nil {
		t.Fatal(err)
	}
	if settings["rtkEnabled"] != false {
		t.Fatalf("rtkEnabled = %v", settings["rtkEnabled"])
	}
	if settings["cavemanEnabled"] != true {
		t.Fatalf("cavemanEnabled = %v", settings["cavemanEnabled"])
	}
	if settings["cavemanLevel"] != "ultra" {
		t.Fatalf("cavemanLevel = %v", settings["cavemanLevel"])
	}
	if n, ok := settings["headroomTimeoutMs"].(float64); !ok || int(n) != 1500 {
		t.Fatalf("headroomTimeoutMs = %v (%T)", settings["headroomTimeoutMs"], settings["headroomTimeoutMs"])
	}
}

// TestApplyTunnelDashboardAccess pins the setting that opens the dashboard on
// a tunnel hostname.
//
// The tunnel gate refuses a login or a dashboard page that arrives through the
// tunnel's own hostname unless this setting is on. A deployment whose only
// way in is the tunnel - a Colab notebook behind a named tunnel - cannot
// reach its dashboard at all without it, and the symptom is a 403 that looks
// like a wrong password.
func TestApplyTunnelDashboardAccess(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	t.Setenv("ROUTER_TUNNEL_DASHBOARD_ACCESS", "true")

	repo := newTestRepo(t)
	Apply(repo)

	settings, err := repo.GetSettingsRaw()
	if err != nil {
		t.Fatal(err)
	}
	if settings["tunnelDashboardAccess"] != true {
		t.Fatalf("tunnelDashboardAccess = %v, want true", settings["tunnelDashboardAccess"])
	}
}

func TestApplyRejectsInvalidSettingsJSON(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	t.Setenv("ROUTER_SETTINGS_JSON", "{not valid json")

	repo := newTestRepo(t)
	before, _ := repo.GetSettingsRaw()

	Apply(repo)

	after, err := repo.GetSettingsRaw()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("Apply changed settings when it should have been a no-op: %d -> %d", len(before), len(after))
	}
}

func TestApplyIsNoOpWhenNothingToInject(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	repo := newTestRepo(t)

	before, _ := repo.GetSettingsRaw()
	Apply(repo)
	after, _ := repo.GetSettingsRaw()

	if len(before) != len(after) {
		t.Fatalf("Apply changed settings when it should have been a no-op: %d -> %d", len(before), len(after))
	}
	keys, _ := repo.GetApiKeys()
	if len(keys) != 0 {
		t.Fatalf("no API key should be seeded without env input, got %d", len(keys))
	}
}