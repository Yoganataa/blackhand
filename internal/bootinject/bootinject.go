package bootinject

// Package bootinject applies environment-driven configuration to the database-backed settings and
// API keys at startup, so a fresh runtime (e.g. Google Colab, where DATA_DIR is ephemeral) can be
// reproduced from environment variables alone instead of dashboard clicks.
//
// Precedence: environment wins over the persisted row, per key. Idempotent: the same env yields the
// same state. Injected API keys are tracked by a deterministic id derived from the key value, so
// re-running never duplicates a key, and keys created in the dashboard are never removed.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"9router/proxy/internal/db"
)

// Apply runs all injectors. It never returns an error: failures are logged and the server keeps
// starting, so a typo in one env var cannot brick a runtime.
//
// The signature matches an fx.Invoke target: it takes the repository and returns an error, so the
// application can run it as a startup side effect before anything reads the settings row.
func Apply(repo *db.Repo) error {
	applyAPIKeys(repo)
	applySettings(repo)
	return nil
}

// ---- API keys ---------------------------------------------------------------

func applyAPIKeys(repo *db.Repo) {
	keys := collectAPIKeys()
	if len(keys) == 0 {
		return
	}
	seeded := 0
	for _, key := range keys {
		id := "env-" + shortHash(key.Value)
		existing, err := repo.GetApiKeyByKey(key.Value)
		if err != nil {
			log.Printf("[bootinject] could not look up API key %s: %v", key.Name, err)
			continue
		}
		if existing != nil {
			continue // already present; do not overwrite dashboard edits
		}
		if err := repo.CreateApiKey(id, key.Value, key.Name, ""); err != nil {
			log.Printf("[bootinject] could not seed API key %s: %v", key.Name, err)
			continue
		}
		seeded++
	}
	if seeded > 0 {
		log.Printf("[bootinject] seeded %d API key(s) from environment", seeded)
	}
}

type injectedKey struct {
	Value string
	Name  string
}

func collectAPIKeys() []injectedKey {
	name := strings.TrimSpace(os.Getenv("ROUTER_API_KEY_NAME"))
	if name == "" {
		name = "env"
	}
	result := []injectedKey{}
	seen := map[string]bool{}

	add := func(value, overrideName string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		entryName := name
		if overrideName != "" {
			entryName = overrideName
		}
		result = append(result, injectedKey{Value: value, Name: entryName})
	}

	if v := os.Getenv("ROUTER_API_KEY"); v != "" {
		add(v, "")
	}
	if v := os.Getenv("NINEROUTER_API_KEY"); v != "" {
		add(v, "")
	}
	if v := os.Getenv("ROUTER_API_KEYS"); v != "" {
		for _, part := range strings.Split(v, ",") {
			add(part, "")
		}
	}
	if v := os.Getenv("ROUTER_API_KEYS_JSON"); v != "" {
		var payload any
		if err := json.Unmarshal([]byte(v), &payload); err != nil {
			log.Printf("[bootinject] ROUTER_API_KEYS_JSON invalid: %v", err)
		} else {
			switch typed := payload.(type) {
			case []any:
				for _, item := range typed {
					switch entry := item.(type) {
					case string:
						add(entry, "")
					case map[string]any:
						value, _ := entry["key"].(string)
						entryName, _ := entry["name"].(string)
						add(value, entryName)
					}
				}
			case map[string]any:
				for key, value := range typed {
					if text, ok := value.(string); ok {
						add(text, key)
					}
				}
			}
		}
	}
	return result
}

// ---- settings ---------------------------------------------------------------

func applySettings(repo *db.Repo) {
	current, err := repo.GetSettingsRaw()
	if err != nil || current == nil {
		current = map[string]any{}
	}

	injected := map[string]any{}
	if raw := strings.TrimSpace(os.Getenv("ROUTER_SETTINGS_JSON")); raw != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			log.Printf("[bootinject] ROUTER_SETTINGS_JSON invalid: %v", err)
		} else {
			for key, value := range payload {
				injected[key] = value
			}
		}
	}

	// Sugar variables, only when present, so the notebook can stay flat.
	sugar := map[string]any{}
	setBool := func(envKey, settingKey string) {
		if v := os.Getenv(envKey); v != "" {
			if parsed, err := strconv.ParseBool(v); err == nil {
				sugar[settingKey] = parsed
			} else {
				log.Printf("[bootinject] %s is not a valid boolean: %v", envKey, err)
			}
		}
	}
	setString := func(envKey, settingKey string) {
		if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
			sugar[settingKey] = v
		}
	}
	setInt := func(envKey, settingKey string) {
		if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				sugar[settingKey] = parsed
			} else {
				log.Printf("[bootinject] %s is not a valid integer: %v", envKey, err)
			}
		}
	}

	setBool("ROUTER_REQUIRE_LOGIN", "requireLogin")
	setBool("ROUTER_REQUIRE_API_KEY", "requireApiKey")
	setBool("ROUTER_RTK_ENABLED", "rtkEnabled")
	setBool("ROUTER_CAVEMAN_ENABLED", "cavemanEnabled")
	setBool("ROUTER_PONYTAIL_ENABLED", "ponytailEnabled")
	setBool("ROUTER_ADHD_ENABLED", "adhdEnabled")
	setBool("ROUTER_HEADROOM_ENABLED", "headroomEnabled")
	setBool("ROUTER_INJECTION_GUARD_ENABLED", "injectionGuardEnabled")
	// The tunnel gate blocks the dashboard on a tunnel hostname unless dashboard
	// access is enabled there, and the gate reads this setting per request. A
	// deployment whose only way in is the tunnel (a Colab notebook behind a
	// named tunnel) therefore cannot reach the dashboard at all without it.
	setBool("ROUTER_TUNNEL_DASHBOARD_ACCESS", "tunnelDashboardAccess")
	setString("ROUTER_CAVEMAN_LEVEL", "cavemanLevel")
	setString("ROUTER_PONYTAIL_LEVEL", "ponytailLevel")
	setString("ROUTER_ADHD_LEVEL", "adhdLevel")
	setString("ROUTER_HEADROOM_URL", "headroomUrl")
	setInt("ROUTER_HEADROOM_TIMEOUT_MS", "headroomTimeoutMs")

	// Deep merge: injected values win over the persisted row, key by key.
	merged := map[string]any{}
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range sugar {
		merged[key] = value
	}
	for key, value := range injected {
		merged[key] = value
	}

	if len(sugar) == 0 && len(injected) == 0 {
		return
	}
	if err := repo.UpdateSettingsRaw(merged); err != nil {
		log.Printf("[bootinject] could not apply injected settings: %v", err)
		return
	}
	log.Printf("[bootinject] applied %d settings from environment", len(sugar)+len(injected))
}

// ---- helpers ----------------------------------------------------------------

func shortHash(value string) string {
	h := uint64(5381)
	for _, c := range value {
		h = h*33 + uint64(c)
	}
	return fmt.Sprintf("%x", h)[:8]
}