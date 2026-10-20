package cloudflared

import (
	"os"
	"path/filepath"
	"time"
)

// ConfigFromEnv builds a cloudflared.Config from the environment. The mode defaults to "off"
// so the named tunnel is opt-in: a Colab notebook (or any deployment) enables it by setting
// CF_TUNNEL_MODE=named and supplying either file paths or inline material.
func ConfigFromEnv(dataDir string) Config {
	get := func(key string) string { return os.Getenv(key) }

	cfg := Config{
		Mode:             get("CF_TUNNEL_MODE"),
		Bin:              get("CF_TUNNEL_BIN"),
		Name:             get("CF_TUNNEL_NAME"),
		ConfigYAML:       get("CF_TUNNEL_CONFIG"),
		ConfigFile:       get("CF_TUNNEL_CONFIG_FILE"),
		Credentials:      get("CF_TUNNEL_CREDENTIALS"),
		CredentialsFile:  get("CF_TUNNEL_CREDENTIALS_FILE"),
		URL:              get("CF_TUNNEL_URL"),
		OriginURL:        get("CF_TUNNEL_ORIGIN_URL"),
		ReadinessTimeout: 30 * time.Second,
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeOff
	}
	if cfg.Bin == "" {
		cfg.Bin = "cloudflared"
	}
	if cfg.WorkDir == "" {
		if dataDir == "" {
			dataDir = os.Getenv("DATA_DIR")
		}
		if dataDir == "" {
			dataDir = "."
		}
		cfg.WorkDir = filepath.Join(dataDir, "cloudflared")
	}
	return cfg
}