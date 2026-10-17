package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Yoganataa/blackhand/internal/config"
)

type Provider struct {
	cfg config.Config
	discoveryClient *http.Client
	upstreamClient *http.Client
	mu sync.Mutex
	cached []Model
	cacheAt time.Time
}
type Model struct {
	ID string `json:"id"`
	Object string `json:"object,omitempty"`
	Created int64 `json:"created,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}
type modelResponse struct { Data []Model `json:"data"` }

func New(cfg config.Config) *Provider {
	return &Provider{cfg: cfg, discoveryClient: &http.Client{Timeout: cfg.DiscoveryTimeout}, upstreamClient: &http.Client{Timeout: cfg.UpstreamTimeout}}
}
func (p *Provider) Models(ctx context.Context) ([]Model, error) {
	p.mu.Lock()
	if len(p.cached) > 0 && time.Since(p.cacheAt) < p.cfg.ModelCacheTTL {
		result := append([]Model(nil), p.cached...); p.mu.Unlock(); return prefixModels(result), nil
	}
	p.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.ModelsURL, nil)
	if err != nil { return nil, fmt.Errorf("build model discovery request: %w", err) }
	if p.cfg.UpstreamAPIKey != "" { req.Header.Set("Authorization", "Bearer "+p.cfg.UpstreamAPIKey) }
	resp, err := p.discoveryClient.Do(req)
	if err != nil { return p.staleOrError(fmt.Errorf("model discovery: %w", err)) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return p.staleOrError(fmt.Errorf("model discovery returned HTTP %d", resp.StatusCode))
	}
	var payload modelResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil { return p.staleOrError(fmt.Errorf("decode model discovery: %w", err)) }
	for i := range payload.Data {
		payload.Data[i].ID = strings.TrimPrefix(payload.Data[i].ID, "oc/")
		if payload.Data[i].Object == "" { payload.Data[i].Object = "model" }
		if payload.Data[i].OwnedBy == "" { payload.Data[i].OwnedBy = "opencode" }
	}
	p.mu.Lock(); p.cached = append([]Model(nil), payload.Data...); p.cacheAt = time.Now(); p.mu.Unlock()
	return prefixModels(payload.Data), nil
}
func (p *Provider) staleOrError(err error) ([]Model, error) {
	p.mu.Lock(); defer p.mu.Unlock()
	if len(p.cached) > 0 { return prefixModels(p.cached), nil }
	return nil, err
}
func prefixModels(models []Model) []Model {
	out := make([]Model, len(models)); copy(out, models)
	for i := range out { if !strings.HasPrefix(out[i].ID, "oc/") { out[i].ID = "oc/" + out[i].ID } }
	return out
}
func (p *Provider) ProxyChat(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimRight(p.cfg.UpstreamBaseURL, "/") + "/chat/completions"
	body, err := io.ReadAll(r.Body)
	if err != nil { http.Error(w, "invalid request body", http.StatusRequestEntityTooLarge); return }

	// Keep the public 9Router model namespace while sending the provider's native model ID upstream.
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) == nil {
		var model string
		if raw, ok := payload["model"]; ok && json.Unmarshal(raw, &model) == nil && strings.HasPrefix(model, "oc/") {
			encoded, marshalErr := json.Marshal(strings.TrimPrefix(model, "oc/"))
			if marshalErr == nil {
				payload["model"] = encoded
				if rewritten, rewriteErr := json.Marshal(payload); rewriteErr == nil { body = rewritten }
			}
		}
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil { http.Error(w, "invalid upstream request", http.StatusBadGateway); return }
	req.ContentLength = int64(len(body))
	for _, h := range []string{"Content-Type", "Accept"} { if v := r.Header.Get(h); v != "" { req.Header.Set(h, v) } }
	if p.cfg.UpstreamAPIKey != "" { req.Header.Set("Authorization", "Bearer "+p.cfg.UpstreamAPIKey) }
	resp, err := p.upstreamClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) { return }
		http.Error(w, "upstream request failed", http.StatusBadGateway); return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Cache-Control", "X-Request-Id"} { if v := resp.Header.Get(h); v != "" { w.Header().Set(h, v) } }
	w.WriteHeader(resp.StatusCode)
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil { return }
			if canFlush { flusher.Flush() }
		}
		if readErr != nil { return }
	}
}
