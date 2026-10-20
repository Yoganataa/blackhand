package dashboard

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/tunnel/cloudflared"
)

// RegisterCloudflareTunnelRoutes mounts the routes that start and stop the
// Cloudflare tunnel the gateway supervises itself.
//
// The routes live in their own file, and the call site in the route table is a
// single line, on purpose: the route tables are the files upstream edits most,
// and a two-line insertion there is a merge conflict waiting to happen. The
// call site is also placed at the end of the table rather than next to the
// other tunnel routes, so an upstream edit to that block lands far enough away
// to merge cleanly.
//
// prefix is the path the routes hang off: "" inside a group that is already
// mounted at /api, "/api" when the caller registers on the root router.
//
// The tunnel mode comes from the environment (CF_TUNNEL_MODE), so this is a
// start/stop control for a tunnel the operator configured, not a way to create
// one. With no tunnel configured the mode is "off" and enabling reports which
// environment variables to set.
func RegisterCloudflareTunnelRoutes(r chi.Router, h *DashboardHandler, prefix string) {
	r.Post(prefix+"/tunnel/cloudflare-enable", h.HandleCloudflareTunnelEnable)
	r.Post(prefix+"/tunnel/cloudflare-disable", h.HandleCloudflareTunnelDisable)
}

// HandleCloudflareTunnelEnable starts the supervised cloudflared process and
// records the resulting public URL in the settings.
func (h *DashboardHandler) HandleCloudflareTunnelEnable(w http.ResponseWriter, r *http.Request) {
	sup := h.tunnelSupervisor()
	if sup.Status().Mode == cloudflared.ModeOff {
		writePlainError(w, http.StatusBadRequest, "Cloudflare tunnel is not configured: set CF_TUNNEL_MODE=named (with CF_TUNNEL_CONFIG/CF_TUNNEL_CREDENTIALS) or CF_TUNNEL_MODE=quick")
		return
	}
	if err := sup.Start(r.Context()); err != nil {
		handlerutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	st := sup.Status()
	_ = h.Repo.UpdateSettingsRaw(map[string]any{
		"tunnelEnabled": true,
		"tunnelUrl":     st.URL,
	})
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"tunnelUrl": st.URL,
		"running":   st.Running,
		"pid":       st.PID,
	})
}

// HandleCloudflareTunnelDisable stops the supervised cloudflared process.
func (h *DashboardHandler) HandleCloudflareTunnelDisable(w http.ResponseWriter, r *http.Request) {
	h.tunnelSupervisor().Stop()
	_ = h.Repo.UpdateSettingsRaw(map[string]any{
		"tunnelEnabled": false,
	})
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}
