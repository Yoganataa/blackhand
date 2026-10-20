package app_test

import (
	"path/filepath"
	"testing"

	"9router/proxy/internal/app"
	"9router/proxy/internal/bootinject"
	"9router/proxy/internal/db"
)

// TestBootInjectRunsBeforeTokenSaverConfig pins the ordering between the
// environment injection and the token-saver configuration.
//
// The injection writes the settings row, and ProvideTokenSaverConfig builds
// the running configuration from that row. When the injection ran later, the
// row said rtkEnabled=false while the process still had RTK on: the setting
// was stored, and ignored. The fix is the module order in AppModule, and this
// test is what keeps it in place.
func TestBootInjectRunsBeforeTokenSaverConfig(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	t.Setenv("ROUTER_API_KEY", "")
	t.Setenv("ROUTER_SETTINGS_JSON", `{"rtkEnabled":false,"cavemanEnabled":true}`)

	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "data.sqlite"))
	if err != nil {
		t.Fatalf("OpenDatabase: %v", err)
	}
	defer conn.Close()
	if err := db.EnsureCoreSchema(conn); err != nil {
		t.Fatalf("EnsureCoreSchema: %v", err)
	}
	repo := db.NewRepo(conn)

	// The injection runs first, exactly as the app's module order does it.
	if err := bootinject.Apply(repo); err != nil {
		t.Fatalf("bootinject.Apply: %v", err)
	}

	tsc := app.ProvideTokenSaverConfig(repo, app.CLIParams{})
	if tsc.RTKEnabled() {
		t.Error("RTK must be off: the injected rtkEnabled=false was not read")
	}
	if !tsc.CavemanEnabled() {
		t.Error("Caveman must be on: the injected cavemanEnabled=true was not read")
	}
}
