package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Per-provider custom header overrides, mirroring 9Router's /api/providers/[id]/overrides:
// user overrides live in settings.providerOverrides[providerId].headers, and the effective set is the
// registry's built-in transport headers with the overrides layered on top.
const (
	maxOverrideHeaders    = 20
	maxOverrideValueBytes = 8192
)

// Request-structure and auth headers a user override must never touch.
var blockedOverrideHeaders = map[string]bool{
	"host":              true,
	"content-length":    true,
	"content-type":      true,
	"connection":        true,
	"transfer-encoding": true,
	"authorization":     true,
	"cookie":            true,
}

// builtinProviderHeaders is the registry subset: the transport headers this port sends upstream for a
// provider, copied from 9Router `open-sse/providers/registry/<id>.js` → transport.headers.
var builtinProviderHeaders = map[string]map[string]string{
	"opencode": {"x-opencode-client": "desktop"},
}

// RFC 7230 token subset: letters, digits, hyphen.
var overrideHeaderNameRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// normalizeProviderOverride validates a user payload. Empty input clears the override (nil, "").
func normalizeProviderOverride(headers map[string]any) (map[string]string, string) {
	if len(headers) == 0 {
		return nil, ""
	}
	if len(headers) > maxOverrideHeaders {
		return nil, fmt.Sprintf("Too many headers (max %d)", maxOverrideHeaders)
	}
	clean := map[string]string{}
	for name, raw := range headers {
		value, isString := raw.(string)
		if !isString || value == "" {
			continue
		}
		if !overrideHeaderNameRe.MatchString(name) {
			return nil, "Invalid header name: " + name
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, "Invalid value for header " + name
		}
		if len(value) > maxOverrideValueBytes {
			return nil, fmt.Sprintf("Header %s value too long (max %d)", name, maxOverrideValueBytes)
		}
		if blockedOverrideHeaders[strings.ToLower(name)] {
			return nil, "Header " + name + " cannot be overridden"
		}
		clean[name] = value
	}
	if len(clean) == 0 {
		return nil, ""
	}
	return clean, ""
}

func providerOverridesFrom(settings map[string]any) map[string]any {
	overrides, _ := settings["providerOverrides"].(map[string]any)
	if overrides == nil {
		return map[string]any{}
	}
	return overrides
}

func (a *API) providerOverridesGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	settings, err := a.store.GetSettings(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to get overrides"})
		return
	}
	headers := map[string]string{}
	if entry, ok := providerOverridesFrom(settings)[id].(map[string]any); ok {
		if stored, ok := entry["headers"].(map[string]any); ok {
			for name, value := range stored {
				if text, ok := value.(string); ok {
					headers[name] = text
				}
			}
		}
	}
	builtin := builtinProviderHeaders[id]
	if builtin == nil {
		builtin = map[string]string{}
	}
	writeJSON(w, 200, map[string]any{"headers": headers, "builtinHeaders": builtin})
}

func (a *API) providerOverridesPut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Headers map[string]any `json:"headers"`
	}
	if err := decodeJSON(w, r, &body, 1<<20); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid JSON"})
		return
	}
	clean, message := normalizeProviderOverride(body.Headers)
	if message != "" {
		writeJSON(w, 400, map[string]string{"error": message})
		return
	}

	settings, err := a.store.GetSettings(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to save overrides"})
		return
	}
	overrides := map[string]any{}
	for key, value := range providerOverridesFrom(settings) {
		overrides[key] = value
	}
	saved := map[string]string{}
	if clean != nil {
		entry := map[string]any{"headers": map[string]any{}}
		for name, value := range clean {
			entry["headers"].(map[string]any)[name] = value
			saved[name] = value
		}
		overrides[id] = entry
	} else {
		delete(overrides, id)
	}
	if _, err := a.store.UpdateSettings(r.Context(), map[string]any{"providerOverrides": overrides}); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to save overrides"})
		return
	}
	a.appendLog("Provider header overrides updated: " + id)
	writeJSON(w, 200, map[string]any{"headers": saved})
}
