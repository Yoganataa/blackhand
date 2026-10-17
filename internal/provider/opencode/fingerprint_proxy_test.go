package opencode

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yoganataa/blackhand/internal/config"
)

// Pins what this port actually puts on the wire for a free-tier model: the inference lane, the v2
// client fingerprint headers, and the body fingerprint (tool definitions plus stream:true).
func TestProxyChatAppliesFreeTierFingerprint(t *testing.T) {
	var gotPath string
	var gotHeader http.Header
	var gotBody string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"stub"}`))
	}))
	defer upstream.Close()

	provider := New(config.Config{UpstreamBaseURL: upstream.URL + "/zen/v1", UpstreamTimeout: 10 * time.Second})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"oc/mimo-v2.6-flash-free","messages":[{"role":"user","content":"ping"}],"stream":false}`))
	recorder := httptest.NewRecorder()

	result := provider.ProxyChatWithOptions(recorder, request, ProxyOptions{})
	if result.Error != "" {
		t.Fatalf("proxy error: %s (status %d)", result.Error, result.StatusCode)
	}
	if gotPath != "/inference/v1/chat/completions" {
		t.Fatalf("upstream path = %q, want the inference lane", gotPath)
	}
	if gotHeader.Get("User-Agent") != openCodeUserAgent {
		t.Fatalf("User-Agent = %q, want %q", gotHeader.Get("User-Agent"), openCodeUserAgent)
	}
	if gotHeader.Get("x-opencode-client") != "cli" {
		t.Fatalf("x-opencode-client = %q", gotHeader.Get("x-opencode-client"))
	}
	if gotHeader.Get("x-opencode-project") != "global" {
		t.Fatalf("x-opencode-project = %q", gotHeader.Get("x-opencode-project"))
	}
	if gotHeader.Get("Authorization") != "Bearer public" {
		t.Fatalf("Authorization = %q, want the free-tier bearer", gotHeader.Get("Authorization"))
	}
	session := gotHeader.Get("x-opencode-session")
	if session == "" || gotHeader.Get("x-opencode-session-id") != session ||
		gotHeader.Get("x-session-affinity") != session || gotHeader.Get("x-session-id") != session {
		t.Fatalf("session header aliases not aligned: %q / %q / %q / %q",
			session, gotHeader.Get("x-opencode-session-id"), gotHeader.Get("x-session-affinity"), gotHeader.Get("x-session-id"))
	}
	if !strings.Contains(gotBody, `"model":"mimo-v2.6-flash-free"`) {
		t.Fatalf("model prefix not stripped: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"stream":true`) {
		t.Fatalf("free-tier request not forced to stream: %s", gotBody)
	}
	for _, name := range []string{`"read"`, `"shell"`} {
		if !strings.Contains(gotBody, name) {
			t.Fatalf("fingerprint tool %s missing from the body: %s", name, gotBody)
		}
	}
}

// A paid model keeps the zen lane and no free-tier body fingerprint.
func TestProxyChatKeepsPaidModelOnZenLane(t *testing.T) {
	var gotPath string
	var gotBody string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"stub"}`))
	}))
	defer upstream.Close()

	provider := New(config.Config{UpstreamBaseURL: upstream.URL + "/zen/v1", UpstreamTimeout: 10 * time.Second})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"oc/claude-opus-5","messages":[{"role":"user","content":"ping"}]}`))
	recorder := httptest.NewRecorder()

	if result := provider.ProxyChatWithOptions(recorder, request, ProxyOptions{}); result.Error != "" {
		t.Fatalf("proxy error: %s", result.Error)
	}
	if gotPath != "/zen/v1/chat/completions" {
		t.Fatalf("upstream path = %q, want the zen lane", gotPath)
	}
	if strings.Contains(gotBody, `"shell"`) {
		t.Fatalf("free-tier tool fingerprint leaked into a paid request: %s", gotBody)
	}
}
