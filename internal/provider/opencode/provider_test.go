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

func TestProxyChatStripsPublicModelPrefix(t *testing.T) {
	var forwardedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected upstream path %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
			http.Error(w, "read failed", http.StatusInternalServerError)
			return
		}
		var request struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode forwarded body: %v", err)
			http.Error(w, "decode failed", http.StatusBadRequest)
			return
		}
		forwardedModel = request.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"test-response"}`)
	}))
	defer upstream.Close()

	provider := New(config.Config{
		UpstreamBaseURL: upstream.URL,
		UpstreamTimeout: 2 * time.Second,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"oc/example-free-model","messages":[]}`))
	recorder := httptest.NewRecorder()
	provider.ProxyChat(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if forwardedModel != "example-free-model" {
		t.Fatalf("expected native model ID, got %q", forwardedModel)
	}
}

func TestPrefixModelsIsStable(t *testing.T) {
	got := prefixModels([]Model{{ID: "example-free"}, {ID: "oc/already-prefixed"}})
	if got[0].ID != "oc/example-free" || got[1].ID != "oc/already-prefixed" {
		t.Fatalf("unexpected model prefixes: %#v", got)
	}
}
