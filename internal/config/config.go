package config

import (
	"net"
	"os"
	"strings"
	"time"
)

type Config struct {
	Host string
	Port string
	DataDir string
	APIKey string
	ModelsURL string
	UpstreamBaseURL string
	UpstreamAPIKey string
	DiscoveryTimeout time.Duration
	UpstreamTimeout time.Duration
	ModelCacheTTL time.Duration
}
func Load() Config {
	return Config{
		Host: env("HOST", "127.0.0.1"), Port: env("PORT", "20128"),
		DataDir: env("DATA_DIR", "./data"),
		APIKey: strings.TrimSpace(os.Getenv("NINEROUTER_API_KEY")),
		ModelsURL: env("OPENCODE_MODELS_URL", "https://opencode.ai/zen/v1/models"),
		UpstreamBaseURL: strings.TrimRight(env("UPSTREAM_BASE_URL", "https://opencode.ai/zen/v1"), "/"),
		UpstreamAPIKey: strings.TrimSpace(os.Getenv("UPSTREAM_API_KEY")),
		DiscoveryTimeout: durationEnv("MODEL_DISCOVERY_TIMEOUT", 5*time.Second),
		UpstreamTimeout: durationEnv("UPSTREAM_TIMEOUT", 10*time.Minute),
		ModelCacheTTL: durationEnv("MODEL_CACHE_TTL", 5*time.Minute),
	}
}
func (c Config) Address() string { return net.JoinHostPort(c.Host, c.Port) }
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" { return value }
	return fallback
}
func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" { return fallback }
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 { return fallback }
	return d
}
