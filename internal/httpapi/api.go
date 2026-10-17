package httpapi

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/Yoganataa/blackhand/internal/config"
	"github.com/Yoganataa/blackhand/internal/provider/opencode"
	"github.com/Yoganataa/blackhand/internal/storage"
)

//go:embed all:assets
var webAssets embed.FS

type API struct { cfg config.Config; store *storage.Store; logger *slog.Logger; provider *opencode.Provider }
func New(cfg config.Config, store *storage.Store, logger *slog.Logger) *API {
	return &API{cfg: cfg, store: store, logger: logger, provider: opencode.New(cfg)}
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /readyz", a.readiness)
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /v1/models", a.models)
	mux.HandleFunc("POST /v1/chat/completions", a.chat)
	mux.HandleFunc("GET /api/keys", a.keysInfo)
	mux.HandleFunc("GET /api/settings", a.settings)
	mux.Handle("/", a.staticHandler())
	return a.requestLogger(securityHeaders(mux))
}
func (a *API) health(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) }
func (a *API) readiness(w http.ResponseWriter, r *http.Request) {
	if a.store == nil || a.store.DB.PingContext(r.Context()) != nil {
		writeJSON(w, 503, map[string]any{"ok": false, "ready": false}); return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "ready": true})
}
func (a *API) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.APIKey == "" { next(w, r); return }
		provided := strings.TrimSpace(r.Header.Get("Authorization"))
		if len(provided) >= 7 && strings.EqualFold(provided[:7], "Bearer ") { provided = strings.TrimSpace(provided[7:]) } else { provided = "" }
		if len(provided) != len(a.cfg.APIKey) || subtle.ConstantTimeCompare([]byte(provided), []byte(a.cfg.APIKey)) != 1 {
			writeJSON(w, 401, map[string]any{"error": map[string]string{"message":"Invalid API key","type":"authentication_error"}}); return
		}
		next(w, r)
	}
}
func (a *API) models(w http.ResponseWriter, r *http.Request) {
	a.authorize(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), a.cfg.DiscoveryTimeout); defer cancel()
		models, err := a.provider.Models(ctx)
		if err != nil {
			a.logger.Warn("OpenCode model discovery failed", "error", err)
			writeJSON(w, 502, map[string]any{"error": map[string]string{"message":"Model discovery failed","type":"upstream_error"}}); return
		}
		writeJSON(w, 200, map[string]any{"object":"list","data":models})
	})(w, r)
}
func (a *API) chat(w http.ResponseWriter, r *http.Request) {
	a.authorize(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		a.provider.ProxyChat(w, r)
	})(w, r)
}
func (a *API) keysInfo(w http.ResponseWriter, _ *http.Request) {
	if a.cfg.APIKey == "" { writeJSON(w, 200, map[string]any{"keys":[]any{}, "managed_by":"environment"}); return }
	writeJSON(w, 200, map[string]any{"keys":[]any{map[string]any{"name":"Environment API Key","isActive":true,"source":"environment"}}})
}
func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.DB.QueryContext(r.Context(), "SELECT key, value_json FROM settings ORDER BY key")
	if err != nil { writeJSON(w, 500, map[string]string{"error":"settings query failed"}); return }
	defer rows.Close()
	settings := map[string]json.RawMessage{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil { continue }
		settings[key] = json.RawMessage(value)
	}
	if err := rows.Err(); err != nil { writeJSON(w, 500, map[string]string{"error":"settings query failed"}); return }
	writeJSON(w, 200, map[string]any{"settings":settings})
}
func (a *API) staticHandler() http.Handler {
	root, err := fs.Sub(webAssets, "assets")
	if err != nil { return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "web assets unavailable; run make build", 503) }) }
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		if strings.HasPrefix(clean, "/v1/") || strings.HasPrefix(clean, "/api/") { http.NotFound(w, r); return }
		if clean != "/" {
			if f, err := root.Open(strings.TrimPrefix(clean, "/")); err == nil { _ = f.Close(); files.ServeHTTP(w, r); return }
		}
		index, err := root.Open("index.html")
		if err != nil { http.Error(w, "web app not built; run make build", 503); return }
		defer index.Close()
		info, err := index.Stat()
		if err != nil { http.Error(w, "web app unavailable", 503); return }
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", info.ModTime(), index)
	})
}
func (a *API) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now(); next.ServeHTTP(w, r)
		a.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(payload)
}
