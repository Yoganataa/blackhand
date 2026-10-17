package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yoganataa/blackhand/internal/config"
	"github.com/Yoganataa/blackhand/internal/storage"
)

func testAPI(t *testing.T, key string) *API {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = store.Close() })
	return New(config.Config{APIKey:key}, store, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}
func TestHealthAndReadiness(t *testing.T) {
	a := testAPI(t, "")
	for _, route := range []string{"/healthz", "/readyz", "/api/health"} {
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
		if rec.Code != http.StatusOK { t.Fatalf("%s returned %d", route, rec.Code) }
	}
}
func TestConfiguredAPIKeyProtectsModels(t *testing.T) {
	a := testAPI(t, "test-secret")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusUnauthorized { t.Fatalf("want 401, got %d", rec.Code) }
}
func TestSettingsStartsEmpty(t *testing.T) {
	a := testAPI(t, "")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK { t.Fatalf("want 200, got %d", rec.Code) }
}
