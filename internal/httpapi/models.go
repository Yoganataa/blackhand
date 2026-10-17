package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// The single provider in scope exposes its models under this public prefix.
const openCodeProviderAlias = "oc"

// Model enable/disable state, mirroring 9Router's /api/models/disabled. The disabled set is kept per
// provider alias and is what the provider detail page's model list reads and writes.

func (a *API) modelsDisabledGet(w http.ResponseWriter, r *http.Request) {
	alias := strings.TrimSpace(r.URL.Query().Get("providerAlias"))
	if alias != "" {
		ids, err := a.store.GetDisabledModelsFor(r.Context(), alias)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "Failed to fetch disabled models"})
			return
		}
		writeJSON(w, 200, map[string]any{"ids": ids})
		return
	}
	all, err := a.store.GetDisabledModels(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to fetch disabled models"})
		return
	}
	writeJSON(w, 200, map[string]any{"disabled": all})
}

func (a *API) modelsDisabledCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProviderAlias string   `json:"providerAlias"`
		IDs           []string `json:"ids"`
	}
	if err := decodeJSON(w, r, &body, 1<<20); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid JSON"})
		return
	}
	if strings.TrimSpace(body.ProviderAlias) == "" || body.IDs == nil {
		writeJSON(w, 400, map[string]string{"error": "providerAlias and ids[] required"})
		return
	}
	if err := a.store.DisableModels(r.Context(), strings.TrimSpace(body.ProviderAlias), body.IDs); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to disable models"})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *API) modelsDisabledDelete(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	alias := strings.TrimSpace(query.Get("providerAlias"))
	if alias == "" {
		writeJSON(w, 400, map[string]string{"error": "providerAlias required"})
		return
	}
	ids := []string{}
	if id := strings.TrimSpace(query.Get("id")); id != "" {
		ids = append(ids, id)
	}
	if err := a.store.EnableModels(r.Context(), alias, ids); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to enable models"})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

// modelsCatalogue mirrors 9Router's GET /api/models: the provider's model catalogue, behind the
// dashboard session because the public /v1/models endpoint requires a client API key.
// Deviation from upstream: the disabled set is not filtered out here. The provider detail page needs
// the full list to offer restoring a disabled model and reads /api/models/disabled separately.
func (a *API) modelsCatalogue(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.DiscoveryTimeout)
	defer cancel()

	models, err := a.provider.Models(ctx)
	if err != nil {
		a.logger.Warn("OpenCode model discovery failed", "error", err)
		writeJSON(w, 502, map[string]string{"error": "Failed to fetch models"})
		return
	}
	prefix := openCodeProviderAlias + "/"
	list := make([]map[string]any, 0, len(models))
	for _, model := range models {
		internal := strings.TrimPrefix(model.ID, prefix)
		list = append(list, map[string]any{
			"provider":    openCodeProviderAlias,
			"model":       internal,
			"name":        "",
			"fullModel":   model.ID,
			"routedModel": model.ID,
			"alias":       internal,
		})
	}
	writeJSON(w, 200, map[string]any{"models": list})
}

// internalTestKey returns an active unrestricted key for the loopback test call, or "" when no such
// key exists (the router may also run without key enforcement).
func (a *API) internalTestKey(ctx context.Context) string {
	keys, err := a.store.GetAPIKeys(ctx)
	if err != nil {
		return ""
	}
	for _, key := range keys {
		if key.IsActive && !key.Access.Restricted {
			return key.Key
		}
	}
	return ""
}

// modelTest mirrors 9Router's POST /api/models/test: it pings the model through this router's own
// inference pipeline with a minimal non-streaming completion, so the verdict reflects the same path a
// real request takes (including the configured outbound proxy).
func (a *API) modelTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
		Kind  string `json:"kind"`
	}
	if err := decodeJSON(w, r, &body, 64<<10); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid JSON"})
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		writeJSON(w, 400, map[string]string{"error": "Model required"})
		return
	}

	payload, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     false,
	})
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "Could not encode the test request"})
		return
	}

	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+a.cfg.Address()+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "Could not build the test request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if key := a.internalTestKey(r.Context()); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": "Model not reachable"})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 == 2 {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}

	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &parsed)
	message := strings.TrimSpace(parsed.Error.Message)
	if message == "" {
		message = strings.TrimSpace(string(raw))
	}
	if message == "" {
		message = "Model not reachable"
	}
	writeJSON(w, 200, map[string]any{"ok": false, "error": message})
}
