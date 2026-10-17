package httpapi

import (
	"database/sql"
	"context"
	"encoding/json"
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


func loginCookieForTest(t *testing.T, a *API) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("{\"password\":\"123456\"}"))
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("test login returned %d: %s", rec.Code, rec.Body.String()) }
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == dashboardCookieName { return cookie }
	}
	t.Fatal("test login returned no dashboard cookie")
	return nil
}

func authenticatedRequest(t *testing.T, a *API, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(cookie)
	if body != "" { req.Header.Set("Content-Type", "application/json") }
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	return rec
}

func TestAPIKeyManagementHTTPRoutes(t *testing.T) {
	a := testAPI(t, "")
	if rec := httptest.NewRecorder(); func() bool {
		a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/keys", nil))
		return rec.Code == http.StatusUnauthorized
	}() == false { t.Fatalf("unauthenticated key listing returned %d", rec.Code) }

	cookie := loginCookieForTest(t, a)
	created := authenticatedRequest(t, a, cookie, http.MethodPost, "/api/keys", "{\"name\":\"integration key\"}")
	if created.Code != http.StatusCreated { t.Fatalf("create key returned %d: %s", created.Code, created.Body.String()) }
	var result struct { ID string `json:"id"`; Key string `json:"key"` }
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil { t.Fatal(err) }
	if result.ID == "" || result.Key == "" { t.Fatalf("invalid created key response: %+v", result) }

	restricted := authenticatedRequest(t, a, cookie, http.MethodPut, "/api/keys/"+result.ID, "{\"access\":{\"restricted\":true,\"allow\":[\"oc/example-model\"]}}")
	if restricted.Code != http.StatusOK { t.Fatalf("restrict key returned %d: %s", restricted.Code, restricted.Body.String()) }
	key, err := a.store.GetAPIKeyByID(context.Background(), result.ID)
	if err != nil { t.Fatal(err) }
	if key == nil || !a.store.AllowedForModel(key, "oc/example-model") || a.store.AllowedForModel(key, "oc/other-model") {
		t.Fatalf("HTTP access policy wasn't applied correctly: %+v", key)
	}

	disabled := authenticatedRequest(t, a, cookie, http.MethodPut, "/api/keys/"+result.ID, "{\"isActive\":false}")
	if disabled.Code != http.StatusOK { t.Fatalf("disable key returned %d: %s", disabled.Code, disabled.Body.String()) }
	inference := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	inference.Header.Set("Authorization", "Bearer "+result.Key)
	inferenceRec := httptest.NewRecorder()
	a.Handler().ServeHTTP(inferenceRec, inference)
	if inferenceRec.Code != http.StatusUnauthorized { t.Fatalf("inactive key should get 401 before model discovery, got %d", inferenceRec.Code) }

	deleted := authenticatedRequest(t, a, cookie, http.MethodDelete, "/api/keys/"+result.ID, "")
	if deleted.Code != http.StatusOK { t.Fatalf("delete key returned %d: %s", deleted.Code, deleted.Body.String()) }
}

func TestProxyPoolManagementHTTPRoutes(t *testing.T) {
	a := testAPI(t, "")
	cookie := loginCookieForTest(t, a)
	created := authenticatedRequest(t, a, cookie, http.MethodPost, "/api/proxy-pools",
		"{\"name\":\"test pool\",\"proxyUrl\":\"http://127.0.0.1:8888\",\"type\":\"http\",\"isActive\":true}")
	if created.Code != http.StatusCreated { t.Fatalf("create pool returned %d: %s", created.Code, created.Body.String()) }
	var result struct { ProxyPool struct { ID string `json:"id"` } `json:"proxyPool"` }
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil { t.Fatal(err) }
	if result.ProxyPool.ID == "" { t.Fatalf("missing pool id: %s", created.Body.String()) }

	updated := authenticatedRequest(t, a, cookie, http.MethodPut, "/api/proxy-pools/"+result.ProxyPool.ID, "{\"strictProxy\":true}")
	if updated.Code != http.StatusOK { t.Fatalf("update pool returned %d: %s", updated.Code, updated.Body.String()) }
	var updatedResult struct { ProxyPool struct { StrictProxy bool `json:"strictProxy"` } `json:"proxyPool"` }
	if err := json.Unmarshal(updated.Body.Bytes(), &updatedResult); err != nil { t.Fatal(err) }
	if !updatedResult.ProxyPool.StrictProxy { t.Fatal("strictProxy was not persisted through management API") }

	deleted := authenticatedRequest(t, a, cookie, http.MethodDelete, "/api/proxy-pools/"+result.ProxyPool.ID, "")
	if deleted.Code != http.StatusOK { t.Fatalf("delete pool returned %d: %s", deleted.Code, deleted.Body.String()) }
}


func TestSanitizedExistingDatabaseKeyAuthenticatesThroughClientMiddleware(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	t.Setenv("INITIAL_PASSWORD", "fixture-only-password")
	dataDir := filepath.Join(t.TempDir(), "data")
	dbPath := filepath.Join(dataDir, "db", "data.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "storage", "testdata", "supplied-db-shape-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(string(fixture)); err != nil {
		_ = rawDB.Close()
		t.Fatalf("load sanitized database fixture: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := storage.Open(dataDir)
	if err != nil {
		t.Fatalf("open sanitized database fixture: %v", err)
	}
	defer store.Close()
	a := New(config.Config{}, store, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	var nextCalled bool
	var resolvedKey string
	gate := a.withClientKey(func(w http.ResponseWriter, r *http.Request, key *storage.APIKey, presented string) {
		nextCalled = true
		if key == nil {
			t.Error("client middleware called next without resolving a database key")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resolvedKey = key.Key
		w.WriteHeader(http.StatusNoContent)
	})

	missing := httptest.NewRecorder()
	gate.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("request without API key returned %d, want 401", missing.Code)
	}
	if nextCalled {
		t.Fatal("client middleware allowed request without a required key")
	}

	const fixtureKey = "sk-blackhand-fixture-key-never-use"
	valid := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	valid.Header.Set("Authorization", "Bearer "+fixtureKey)
	validRec := httptest.NewRecorder()
	gate.ServeHTTP(validRec, valid)
	if validRec.Code != http.StatusNoContent {
		t.Fatalf("active stored API key returned %d, want middleware to allow request; body=%s", validRec.Code, validRec.Body.String())
	}
	if !nextCalled || resolvedKey != fixtureKey {
		t.Fatal("active fixture API key was not resolved by the real client-key middleware")
	}

	invalid := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	invalid.Header.Set("Authorization", "Bearer invalid-fixture-key")
	invalidRec := httptest.NewRecorder()
	gate.ServeHTTP(invalidRec, invalid)
	if invalidRec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown API key returned %d, want 401", invalidRec.Code)
	}
}
