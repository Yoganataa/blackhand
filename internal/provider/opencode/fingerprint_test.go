package opencode

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"
)

func TestIsFreeTierModel(t *testing.T) {
	cases := map[string]bool{
		"deepseek-v4.1-flash-free":        true,
		"oc/mimo-v2.6-flash-free":         true,
		"muse-spark-1.3-contributor-free": true,
		"big-pickle":                      false,
		"claude-opus-5":                   false,
		"deepseek-v4.1-flash":             false,
		"wandb/deepseek-v2.5-free-preview": false,
		"":                                false,
	}
	for model, want := range cases {
		if got := isFreeTierModel(model); got != want {
			t.Fatalf("isFreeTierModel(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestFreeTierUpstreamBase(t *testing.T) {
	if got := freeTierUpstreamBase("https://opencode.ai/zen/v1"); got != "https://opencode.ai/inference/v1" {
		t.Fatalf("zen base = %q", got)
	}
	if got := freeTierUpstreamBase("https://example.test/custom"); got != "https://example.test/custom" {
		t.Fatalf("unrelated base rewritten to %q", got)
	}
}

// The upstream decodes the 12-hex prefix: it must be the low 48 bits of ~(timeMs*0x1000 + counter),
// with a small counter. A random prefix is refused with 403 FreeTierError, which is the bug this test
// pins against.
func TestNewSessionIDUsesCanonicalInvertedTimestamp(t *testing.T) {
	pattern := regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		id := newSessionID()
		if !pattern.MatchString(id) {
			t.Fatalf("session id %q does not match the upstream shape", id)
		}
		if seen[id] {
			t.Fatalf("session id %q repeated", id)
		}
		seen[id] = true

		prefix, err := strconv.ParseUint(id[4:16], 16, 64)
		if err != nil {
			t.Fatalf("prefix of %q is not hex: %v", id, err)
		}
		decoded := (^prefix) & sessionPrefixMask
		counter := decoded & 0xfff
		if counter == 0 || counter > 64 {
			t.Fatalf("counter %d for %q is outside the range the upstream tolerates", counter, id)
		}
		if decoded>>12 == 0 {
			t.Fatalf("time field missing for %q", id)
		}
	}
}

func TestApplyFingerprintHeaders(t *testing.T) {
	const session = "ses_abcdefabcdef0123456789abcd"
	header := http.Header{}
	applyFingerprintHeaders(header, "", session, true)
	if header.Get("User-Agent") != openCodeUserAgent {
		t.Fatalf("User-Agent = %q", header.Get("User-Agent"))
	}
	if header.Get("x-opencode-client") != "cli" {
		t.Fatalf("x-opencode-client = %q", header.Get("x-opencode-client"))
	}
	if header.Get("Authorization") != "Bearer public" {
		t.Fatalf("Authorization = %q, want the free-tier public bearer", header.Get("Authorization"))
	}
	if header.Get("Accept") != "text/event-stream" {
		t.Fatalf("Accept = %q for a streaming request", header.Get("Accept"))
	}
	for _, alias := range []string{"x-opencode-session", "x-opencode-session-id", "x-session-affinity", "x-session-id"} {
		if header.Get(alias) != session {
			t.Fatalf("%s = %q", alias, header.Get(alias))
		}
	}
	// The v1 header is refused outright, so it must never be forwarded.
	header.Set("x-opencode-request", "msg_deadbeefdeadbeefdeadbeef")
	applyFingerprintHeaders(header, "", session, false)
	if header.Get("x-opencode-request") != "" {
		t.Fatalf("x-opencode-request survived: %q", header.Get("x-opencode-request"))
	}
	if header.Get("Accept") != "*/*" {
		t.Fatalf("Accept = %q for a non-streaming request", header.Get("Accept"))
	}

	keyed := http.Header{}
	applyFingerprintHeaders(keyed, "upstream-secret", session, true)
	if keyed.Get("Authorization") != "Bearer upstream-secret" {
		t.Fatalf("configured upstream key ignored: %q", keyed.Get("Authorization"))
	}
}

func TestApplyFingerprintTools(t *testing.T) {
	body := map[string]json.RawMessage{
		"model":    json.RawMessage(`"mimo-v2.6-flash-free"`),
		"messages": json.RawMessage(`[{"role":"user","content":"ping"}]`),
		"stream":   json.RawMessage("false"),
	}
	if !applyFingerprintTools(body) {
		t.Fatal("body should have been changed")
	}
	if string(body["stream"]) != "true" {
		t.Fatalf("stream = %s, want true", body["stream"])
	}
	var tools []map[string]any
	if err := json.Unmarshal(body["tools"], &tools); err != nil {
		t.Fatalf("tools not valid JSON: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %d, want the read and shell definitions", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		names[name] = true
		if description, _ := function["description"].(string); description != openCodeToolDefinitions[name]["description"] {
			t.Fatalf("tool %s description was reworded", name)
		}
	}
	if !names["read"] || !names["shell"] {
		t.Fatalf("missing fingerprint tools: %v", names)
	}

	var messages []map[string]any
	if err := json.Unmarshal(body["messages"], &messages); err != nil {
		t.Fatalf("messages not valid JSON: %v", err)
	}
	if len(messages) != 2 || messages[0]["role"] != "system" {
		t.Fatalf("placeholder note not prepended: %v", messages)
	}

	// A body that already declares both tools must not gain duplicates.
	existing := map[string]json.RawMessage{
		"tools": json.RawMessage(`[{"type":"function","function":{"name":"read"}},{"type":"function","function":{"name":"shell"}}]`),
	}
	_ = applyFingerprintTools(existing)
	var kept []map[string]any
	_ = json.Unmarshal(existing["tools"], &kept)
	if len(kept) != 2 {
		t.Fatalf("existing tools duplicated: %v", kept)
	}
}
