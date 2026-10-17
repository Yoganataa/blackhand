package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yoganataa/blackhand/internal/config"
)

func TestRelayPoolForwardsTargetHeaders(t *testing.T) {
	var targetHeader, pathHeader, forwardedModel string
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHeader = r.Header.Get("x-relay-target")
		pathHeader = r.Header.Get("x-relay-path")
		raw, err := io.ReadAll(r.Body)
		if err != nil { t.Errorf("read relay request: %v", err); http.Error(w, "bad request", 400); return }
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil { t.Errorf("decode relay body: %v", err); http.Error(w, "bad request", 400); return }
		forwardedModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"id\":\"relay-response\"}")
	}))
	defer relay.Close()
	provider := New(config.Config{UpstreamBaseURL: "http://origin.example/zen/v1", UpstreamTimeout: 2 * time.Second})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{\"model\":\"oc/free-model\",\"messages\":[]}"))
	recorder := httptest.NewRecorder()
	result := provider.ProxyChatWithOptions(recorder, request, ProxyOptions{RelayURL: relay.URL})
	if recorder.Code != http.StatusOK || result.StatusCode != http.StatusOK {
		t.Fatalf("relay request failed: HTTP %d, result %#v, body %s", recorder.Code, result, recorder.Body.String())
	}
	if targetHeader != "http://origin.example" { t.Fatalf("unexpected relay target %q", targetHeader) }
	if pathHeader != "/zen/v1/chat/completions" { t.Fatalf("unexpected relay path %q", pathHeader) }
	if forwardedModel != "free-model" { t.Fatalf("relay got prefixed model %q", forwardedModel) }
}

func TestNoProxyBypassesConfiguredPool(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"id\":\"direct-response\"}")
	}))
	defer upstream.Close()
	provider := New(config.Config{UpstreamBaseURL: upstream.URL, UpstreamTimeout: 2 * time.Second})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{\"model\":\"oc/free-model\",\"messages\":[]}"))
	recorder := httptest.NewRecorder()
	provider.ProxyChatWithOptions(recorder, request, ProxyOptions{URL: "http://127.0.0.1:1", NoProxy: "127.0.0.1"})
	if recorder.Code != http.StatusOK || requests != 1 { t.Fatalf("noProxy failed: status=%d requests=%d", recorder.Code, requests) }
}

func TestNonStrictProxyFailureFallsBackButStrictDoesNot(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"id\":\"direct-response\"}")
	}))
	defer upstream.Close()
	newRequest := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{\"model\":\"oc/free-model\",\"messages\":[]}"))
	}
	provider := New(config.Config{UpstreamBaseURL: upstream.URL, UpstreamTimeout: 2 * time.Second})
	recorder := httptest.NewRecorder()
	provider.ProxyChatWithOptions(recorder, newRequest(), ProxyOptions{URL: "http://127.0.0.1:1"})
	if recorder.Code != http.StatusOK || requests != 1 { t.Fatalf("non-strict fallback failed: status=%d requests=%d", recorder.Code, requests) }
	recorder = httptest.NewRecorder()
	provider.ProxyChatWithOptions(recorder, newRequest(), ProxyOptions{URL: "http://127.0.0.1:1", Strict: true})
	if recorder.Code != http.StatusBadGateway || requests != 1 { t.Fatalf("strict proxy unexpectedly went direct: status=%d requests=%d", recorder.Code, requests) }
}

func TestShouldBypassHostSupportsSuffixesAndCIDR(t *testing.T) {
	cases := []struct { host, noProxy string; want bool }{
		{"localhost", "localhost", true},
		{"api.example.com", ".example.com", true},
		{"example.com", "example.com", true},
		{"10.1.2.3", "10.0.0.0/8", true},
		{"elsewhere.net", "example.com", false},
	}
	for _, tc := range cases {
		if got := shouldBypassHost(tc.host, tc.noProxy); got != tc.want { t.Errorf("shouldBypassHost(%q,%q)=%v want %v", tc.host, tc.noProxy, got, tc.want) }
	}
}
