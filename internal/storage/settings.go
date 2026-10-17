package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func environmentBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" { return fallback }
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on": return true
	case "0", "false", "no", "off": return false
	default: return fallback
	}
}

// DefaultSettings mirrors src/lib/db/repos/settingsRepo.js. Values are returned
// as a fresh map so a caller cannot mutate shared defaults.
func DefaultSettings() map[string]any {
	return map[string]any{
		"cloudEnabled": false,
		"tunnelEnabled": false,
		"tunnelUrl": "",
		"tunnelProvider": "cloudflare",
		"tailscaleEnabled": false,
		"tailscaleUrl": "",
		"stickyRoundRobinLimit": 3,
		"providerStrategies": map[string]any{},
		"quotaVisibility": map[string]any{},
		"comboStrategy": "fallback",
		"comboStickyRoundRobinLimit": 1,
		"comboStrategies": map[string]any{},
		"capacityAdapter": map[string]any{
			"vision": map[string]any{"enabled":true,"roundRobin":false,"models":[]any{}},
			"pdf": map[string]any{"enabled":false,"roundRobin":false,"models":[]any{}},
			"audioInput": map[string]any{"enabled":true,"roundRobin":false,"models":[]any{}},
			"videoInput": map[string]any{"enabled":false,"roundRobin":false,"models":[]any{}},
		},
		"requireLogin": true,
		"requireApiKey": true,
		"tunnelDashboardAccess": true,
		"authMode": "password",
		"ssoType": "oidc",
		"oidcIssuerUrl": "",
		"oidcClientId": "",
		"oidcClientSecret": "",
		"oidcScopes": "openid profile email",
		"oidcLoginLabel": "Sign in with OIDC",
		"samlEntryPoint": "",
		"samlIssuer": "urn:9router:sp",
		"samlCert": "",
		"samlLoginLabel": "Sign in with SAML SSO",
		"samlAttributeEmail": "email",
		"samlAttributeName": "name",
		"enableObservability": false,
		"observabilityMaxRecords": 1000,
		"observabilityBatchSize": 20,
		"observabilityFlushIntervalMs": 5000,
		"observabilityMaxJsonSize": 5,
		"outboundProxyEnabled": false,
		"outboundProxyUrl": "",
		"outboundNoProxy": "",
		"mitmRouterBaseUrl": "http://localhost:20128",
		"dnsToolEnabled": map[string]any{},
		"rtkEnabled": true,
		"headroomEnabled": environmentBool("HEADROOM_DEFAULT_ENABLED", true),
		"headroomUrl": envDefault("HEADROOM_URL", "http://localhost:8787"),
		"headroomCompressUserMessages": environmentBool("HEADROOM_DEFAULT_COMPRESS_USER_MESSAGES", true),
		"headroomTimeoutMs": 10000,
		"headroomCodeAware": environmentBool("HEADROOM_DEFAULT_CODE_AWARE", true),
		"headroomKompress": environmentBool("HEADROOM_DEFAULT_KOMPRESS", true),
		"cavemanEnabled": false,
		"cavemanLevel": "full",
		"ponytailEnabled": true,
		"ponytailLevel": "full",
		"pxpipeEnabled": false,
		"pxpipeAutoInstall": true,
		"pxpipeMinChars": 25000,
		"pxpipeTimeoutMs": 15000,
		"providerOverrides": map[string]any{},
	}
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" { return value }
	return fallback
}

func (s *Store) GetSettings(ctx context.Context) (map[string]any, error) {
	out := DefaultSettings()
	var raw string
	err := s.DB.QueryRowContext(ctx, "SELECT data FROM settings WHERE id=1").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) { return nil, err }
	if raw != "" {
		var saved map[string]any
		if err := json.Unmarshal([]byte(raw), &saved); err != nil { return nil, fmt.Errorf("decode settings: %w", err) }
		for key, value := range saved { out[key] = value }
	}
	if _, wasSet := out["outboundProxyEnabled"]; !wasSet {
		if url, ok := out["outboundProxyUrl"].(string); ok && strings.TrimSpace(url) != "" { out["outboundProxyEnabled"] = true }
	}
	if adapter, ok := out["capacityAdapter"].(map[string]any); ok {
		for _, group := range []string{"vision","pdf","audioInput","videoInput"} {
			entry, exists := adapter[group]
			if !exists { continue }
			if item, ok := entry.(map[string]any); ok {
				if models, ok := item["models"].([]any); ok {
					for i, model := range models {
						if model == "oc/mimo-v2.5-free" { models[i] = "oc/mimo-v2.6-flash-free" }
					}
				}
			}
		}
	}
	return out, nil
}

func (s *Store) UpdateSettings(ctx context.Context, updates map[string]any) (map[string]any, error) {
	delete(updates, "password")
	delete(updates, "mitmSudoEncrypted")
	if secret, exists := updates["oidcClientSecret"]; exists {
		if strings.TrimSpace(fmt.Sprint(secret)) == "" { delete(updates, "oidcClientSecret") }
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil { return nil, err }
	defer tx.Rollback()
	var raw string
	err = tx.QueryRowContext(ctx, "SELECT data FROM settings WHERE id=1").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) { return nil, err }
	current := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &current); err != nil { return nil, fmt.Errorf("decode settings for update: %w", err) }
	}
	for key, value := range updates { current[key] = value }
	encoded, err := json.Marshal(current)
	if err != nil { return nil, err }
	if _, err := tx.ExecContext(ctx, "INSERT INTO settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(encoded)); err != nil { return nil, err }
	if err := tx.Commit(); err != nil { return nil, err }
	return s.GetSettings(ctx)
}

// SetPasswordHash is intentionally separate: bulk settings PATCH must never be
// allowed to mass-assign a password field.
func (s *Store) SetPasswordHash(ctx context.Context, hash string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	var raw string
	err = tx.QueryRowContext(ctx, "SELECT data FROM settings WHERE id=1").Scan(&raw)
	if err != nil && err.Error() != "sql: no rows in result set" { return err }
	settings := map[string]any{}
	if raw != "" { if err := json.Unmarshal([]byte(raw), &settings); err != nil { return err } }
	settings["password"] = hash
	encoded, err := json.Marshal(settings)
	if err != nil { return err }
	if _, err := tx.ExecContext(ctx, "INSERT INTO settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(encoded)); err != nil { return err }
	return tx.Commit()
}

func (s *Store) PersistentSecret(filename string, bytes int) ([]byte, error) {
	path := s.DataDir + "/auth/" + filename
	if secret, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(secret))) >= bytes*2 {
		decoded, decErr := hex.DecodeString(strings.TrimSpace(string(secret)))
		if decErr == nil && len(decoded) >= bytes { return decoded, nil }
	}
	secret := make([]byte, bytes)
	if _, err := rand.Read(secret); err != nil { return nil, err }
	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0o600); err != nil { return nil, err }
	return secret, nil
}

func (s *Store) MachineID() (string, error) {
	path := s.DataDir + "/machine-id"
	raw, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil { return "", err }
		if err := os.WriteFile(path, []byte(hex.EncodeToString(random)), 0o600); err != nil { return "", err }
		raw = []byte(hex.EncodeToString(random))
	}
	salt := envDefault("MACHINE_ID_SALT", "endpoint-proxy-salt")
	sum := sha256.Sum256([]byte(strings.TrimSpace(string(raw)) + salt))
	return hex.EncodeToString(sum[:])[:16], nil
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339Nano) }
