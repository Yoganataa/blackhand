package httpapi

import (
	"encoding/json"
	"net/http"
)

// credentials never leave the router, mirroring 9Router's toProviderConnectionResponse.
var connectionCredentialFields = []string{"apiKey", "accessToken", "refreshToken", "idToken"}

// providersList mirrors 9Router's GET /api/providers: every stored provider connection with its
// credential fields removed.
func (a *API) providersList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.DB.QueryContext(r.Context(),
		"SELECT id,provider,authType,name,isActive,data,createdAt,updatedAt FROM providerConnections ORDER BY priority,createdAt")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to fetch providers"})
		return
	}
	defer rows.Close()

	list := make([]map[string]any, 0)
	for rows.Next() {
		var id, provider, authType, name, raw, createdAt, updatedAt string
		var isActive int
		if err := rows.Scan(&id, &provider, &authType, &name, &isActive, &raw, &createdAt, &updatedAt); err != nil {
			writeJSON(w, 500, map[string]string{"error": "Failed to read providers"})
			return
		}
		entry := map[string]any{}
		if json.Unmarshal([]byte(raw), &entry) != nil {
			entry = map[string]any{}
		}
		entry["id"] = id
		entry["provider"] = provider
		entry["authType"] = authType
		entry["name"] = name
		entry["isActive"] = isActive != 0
		entry["createdAt"] = createdAt
		entry["updatedAt"] = updatedAt
		for _, field := range connectionCredentialFields {
			delete(entry, field)
		}
		list = append(list, entry)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Failed to read providers"})
		return
	}
	writeJSON(w, 200, map[string]any{"connections": list})
}
