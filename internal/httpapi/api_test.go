package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yoganataa/blackhand/internal/config"
	"github.com/Yoganataa/blackhand/internal/storage"
)

func testAPI(t *testing.T, key string) *API {
	t.Helper()
	t.Setenv("NINEROUTER_API_KEY", key)
	t.Setenv("INITIAL_PASSWORD", "123456")
	store, err := storage.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = store.Close() })
	return New(config.Config{APIKey: key}, store, slog.New(slog.NewTextHandler(os.Stderr, nil)))
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
func TestSettingsRequiresDashboardSession(t *testing.T) {
	a := testAPI(t, "")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusUnauthorized { t.Fatalf("unauthenticated settings request should be 401, got %d", rec.Code) }

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("{\"password\":\"123456\"}"))
	login.RemoteAddr = "127.0.0.1:12345"
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	a.Handler().ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK { t.Fatalf("login returned %d: %s", loginRec.Code, loginRec.Body.String()) }
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 { t.Fatal("login did not issue a session cookie") }
	request := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	request.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, request)
	if rec.Code != http.StatusOK { t.Fatalf("authenticated settings request returned %d: %s", rec.Code, rec.Body.String()) }
}
func TestSettingsStartsEmpty(t *testing.T) {
	a := testAPI(t, "")
	if _, err := a.store.UpdateSettings(context.Background(), map[string]any{"requireLogin": false}); err != nil { t.Fatal(err) }
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK { t.Fatalf("want 200, got %d", rec.Code) }
}
