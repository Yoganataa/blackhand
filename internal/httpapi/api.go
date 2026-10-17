package httpapi

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Yoganataa/blackhand/internal/config"
	"github.com/Yoganataa/blackhand/internal/provider/opencode"
	"github.com/Yoganataa/blackhand/internal/storage"
	"github.com/Yoganataa/blackhand/internal/tokensaver"
)

//go:embed all:assets
var webAssets embed.FS

type API struct {
	cfg config.Config
	store *storage.Store
	logger *slog.Logger
	provider *opencode.Provider
	tokenSaver *tokensaver.Processor

	logMu sync.Mutex
	logs []string
	logSubs map[uint64]chan logEvent
	nextSub uint64

	activeMu sync.Mutex
	active map[string]int
}
type logEvent struct { Type string; Line string }

func New(cfg config.Config, store *storage.Store, logger *slog.Logger) *API {
	return &API{
		cfg:cfg,store:store,logger:logger,provider:opencode.New(cfg),tokenSaver:tokensaver.New(),
		logs:[]string{},logSubs:map[uint64]chan logEvent{},active:map[string]int{},
	}
}
func (a *API) Handler() http.Handler {
	mux:=http.NewServeMux()
	mux.HandleFunc("GET /healthz",a.health)
	mux.HandleFunc("GET /readyz",a.readiness)
	mux.HandleFunc("GET /api/health",a.health)
	mux.HandleFunc("GET /api/auth/session",a.authSession)
	mux.HandleFunc("POST /api/auth/login",a.authLogin)
	mux.HandleFunc("POST /api/auth/logout",a.authLogout)

	mux.Handle("GET /api/keys",a.dashboardAuth(http.HandlerFunc(a.keysList)))
	mux.Handle("POST /api/keys",a.dashboardAuth(http.HandlerFunc(a.keysCreate)))
	mux.Handle("GET /api/keys/{id}",a.dashboardAuth(http.HandlerFunc(a.keyGet)))
	mux.Handle("PUT /api/keys/{id}",a.dashboardAuth(http.HandlerFunc(a.keyUpdate)))
	mux.Handle("DELETE /api/keys/{id}",a.dashboardAuth(http.HandlerFunc(a.keyDelete)))

	mux.Handle("GET /api/settings",a.dashboardAuth(http.HandlerFunc(a.settingsGet)))
	mux.Handle("PATCH /api/settings",a.dashboardAuth(http.HandlerFunc(a.settingsPatch)))

	mux.Handle("GET /api/providers",a.dashboardAuth(http.HandlerFunc(a.providersList)))
	mux.Handle("GET /api/providers/{id}/overrides",a.dashboardAuth(http.HandlerFunc(a.providerOverridesGet)))
	mux.Handle("PUT /api/providers/{id}/overrides",a.dashboardAuth(http.HandlerFunc(a.providerOverridesPut)))

	mux.Handle("GET /api/proxy-pools",a.dashboardAuth(http.HandlerFunc(a.proxyPoolsList)))
	mux.Handle("POST /api/proxy-pools",a.dashboardAuth(http.HandlerFunc(a.proxyPoolCreate)))
	mux.Handle("GET /api/proxy-pools/{id}",a.dashboardAuth(http.HandlerFunc(a.proxyPoolGet)))
	mux.Handle("PUT /api/proxy-pools/{id}",a.dashboardAuth(http.HandlerFunc(a.proxyPoolUpdate)))
	mux.Handle("DELETE /api/proxy-pools/{id}",a.dashboardAuth(http.HandlerFunc(a.proxyPoolDelete)))
	mux.Handle("POST /api/proxy-pools/{id}/test",a.dashboardAuth(http.HandlerFunc(a.proxyPoolTest)))

	mux.Handle("GET /api/usage/history",a.dashboardAuth(http.HandlerFunc(a.usageHistory)))
	mux.Handle("GET /api/usage/stats",a.dashboardAuth(http.HandlerFunc(a.usageStats)))
	mux.Handle("GET /api/usage/chart",a.dashboardAuth(http.HandlerFunc(a.usageChart)))
	mux.Handle("GET /api/usage/logs",a.dashboardAuth(http.HandlerFunc(a.usageLogs)))
	mux.Handle("GET /api/usage/stream",a.dashboardAuth(http.HandlerFunc(a.usageStream)))
	mux.Handle("GET /api/translator/console-logs",a.dashboardAuth(http.HandlerFunc(a.consoleLogsGet)))
	mux.Handle("DELETE /api/translator/console-logs",a.dashboardAuth(http.HandlerFunc(a.consoleLogsClear)))
	mux.Handle("GET /api/translator/console-logs/stream",a.dashboardAuth(http.HandlerFunc(a.consoleLogsStream)))

	mux.HandleFunc("GET /v1/models",a.models)
	mux.HandleFunc("POST /v1/chat/completions",a.chat)
	mux.Handle("/",a.staticHandler())
	return a.requestLogger(securityHeaders(mux))
}
func (a *API) health(w http.ResponseWriter,_ *http.Request){writeJSON(w,200,map[string]any{"ok":true})}
func (a *API) readiness(w http.ResponseWriter,r *http.Request){
	if a.store==nil||a.store.DB.PingContext(r.Context())!=nil{writeJSON(w,503,map[string]any{"ok":false,"ready":false});return}
	writeJSON(w,200,map[string]any{"ok":true,"ready":true})
}
func (a *API) staticHandler()http.Handler{
	root,err:=fs.Sub(webAssets,"assets")
	if err!=nil{return http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){http.Error(w,"web assets unavailable; run make build",503)})}
	files:=http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		clean:=path.Clean("/"+r.URL.Path)
		if strings.HasPrefix(clean,"/v1/")||strings.HasPrefix(clean,"/api/"){http.NotFound(w,r);return}
		if clean!="/" {if f,err:=root.Open(strings.TrimPrefix(clean,"/"));err==nil{_ = f.Close();files.ServeHTTP(w,r);return}}
		index,err:=root.Open("index.html");if err!=nil{http.Error(w,"web app not built; run make build",503);return};defer index.Close()
		info,err:=index.Stat();if err!=nil{http.Error(w,"web app unavailable",503);return}
		data,err:=io.ReadAll(index);if err!=nil{http.Error(w,"web app unavailable",503);return}
		w.Header().Set("Content-Type","text/html; charset=utf-8")
		http.ServeContent(w,r,"index.html",info.ModTime(),bytes.NewReader(data))
	})
}
func (a *API) requestLogger(next http.Handler)http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		start:=time.Now()
		next.ServeHTTP(w,r)
		line:=time.Now().Format("15:04:05")+" "+r.Method+" "+r.URL.Path+" "+time.Since(start).String()
		a.appendLog(line)
		a.logger.Info("http request","method",r.Method,"path",r.URL.Path,"duration_ms",time.Since(start).Milliseconds())
	})
}
func securityHeaders(next http.Handler)http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		w.Header().Set("X-Content-Type-Options","nosniff")
		w.Header().Set("Referrer-Policy","no-referrer")
		next.ServeHTTP(w,r)
	})
}
func writeJSON(w http.ResponseWriter,status int,payload any){
	w.Header().Set("Content-Type","application/json; charset=utf-8")
	w.Header().Set("Cache-Control","no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
