package opencode

// Free-tier client fingerprint for OpenCode's upstream gate.
//
// Ported from 9Router `open-sse/utils/opencodeFingerprint.js`, `open-sse/executors/opencode.js`
// and PRs #4883 (`refresh the client fingerprint to the 2.0 channel form`) and #4860
// (`align free-tier v2 headers`). Live probes there established that:
//
//   - the User-Agent must use the v2 channel form `opencode/<channel>/<version>/<client>`; the old
//     `opencode/1.18.31` form is answered with 403 FreeTierError,
//   - `x-opencode-client` must be `cli`, and free-tier requests authenticate as `Bearer public`,
//   - free models are served on `https://opencode.ai/inference/v1/*` instead of `/zen/v1/*` and must
//     be requested with `stream: true`,
//   - the body must declare OpenCode's own `read` and `shell` tools with their exact descriptions and
//     parameter schemas; renamed or reworded definitions are rejected.
//
// Everything else (session aliases, project header) follows the v2 header set.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	// OpenCode v2 UA shape: opencode/<channel>/<version>/<client>.
	openCodeUserAgent = "opencode/latest/2.0.29/cli"
	openCodeClient    = "cli"
	openCodeProject   = "global"
	// Free-tier models live on the inference lane; paid models stay on /zen/v1.
	openCodeZenBase       = "/zen/v1"
	openCodeInferenceBase = "/inference/v1"
	sessionHeader         = "x-opencode-session"
	base62Alphabet        = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// isFreeTierModel mirrors 9Router `isOpencodeFreeTierModel`: the base model id ends in `-free`.
func isFreeTierModel(model string) bool {
	base := model
	if index := strings.LastIndex(base, "/"); index >= 0 {
		base = base[index+1:]
	}
	return strings.HasSuffix(strings.ToLower(base), "-free")
}

// newSessionID builds an OpenCode session id: `ses_` + 12 hex + 14 base62 characters, the shape the
// upstream session regex accepts.
func newSessionID() string {
	raw := make([]byte, 6+14)
	if _, err := rand.Read(raw); err != nil {
		return "ses_0000000000000000000000000"
	}
	var builder strings.Builder
	builder.WriteString("ses_")
	builder.WriteString(hex.EncodeToString(raw[:6]))
	for _, value := range raw[6:] {
		builder.WriteByte(base62Alphabet[int(value)%len(base62Alphabet)])
	}
	return builder.String()
}

// applyFingerprintHeaders sets the header set OpenCode's gate classifies on. A configured upstream key
// wins; otherwise the free tier authenticates as `Bearer public`.
func applyFingerprintHeaders(header http.Header, upstreamAPIKey string, sessionID string) {
	header.Set("User-Agent", openCodeUserAgent)
	header.Set("x-opencode-client", openCodeClient)
	header.Set("x-opencode-project", openCodeProject)
	header.Set(sessionHeader, sessionID)
	// v2 alias set, mirroring PR #4860.
	header.Set("x-opencode-session-id", sessionID)
	header.Set("x-session-affinity", sessionID)
	header.Set("x-session-id", sessionID)

	if strings.TrimSpace(upstreamAPIKey) != "" {
		header.Set("Authorization", "Bearer "+upstreamAPIKey)
		return
	}
	header.Set("Authorization", "Bearer public")
}

// openCodeToolDefinitions holds OpenCode's own `read` and `shell` definitions verbatim. The upstream
// gate compares descriptions and parameter schemas, so neither may be reworded.
var openCodeToolDefinitions = map[string]map[string]any{
	"read": {
		"description": "Read the contents of a file or directory. Supports text files, images, and PDFs. Images and PDFs are presented directly to the model. Each text line is prefixed by its 1-based line number as <line>: <content>. The prefix is for reference and is not part of the file content. Directory entries are returned one per line. Use offset and limit to read large files or directories in sections. Prefer one larger read over many small slices, and use grep to find specific content in large files.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "File or directory to read"},
				"offset": map[string]any{"type": "integer", "minimum": 0, "description": "The line or directory entry to start reading from (1-based)"},
				"limit":  map[string]any{"type": "integer", "minimum": 0, "description": "The maximum number of lines or directory entries to read (defaults to and capped at 2000)"},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	},
	"shell": {
		"description": "Execute a shell command and return its output. Commands run on Linux using bash. Quote file paths containing spaces or special characters. Prefer dedicated tools over shell commands when possible. When output is large, the full result is saved to a file and a truncated preview is returned. Rely on automatic truncation unless filtering the output is more useful. Commands accept an optional timeout, background commands have no timeout by default. Background commands return immediately, and you will be notified when they complete.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":    map[string]any{"type": "string", "description": "Shell command string to execute"},
				"workdir":    map[string]any{"type": "string", "description": "Working directory to execute the command in. Defaults to the current working directory. When possible, avoid changing directories in the command and set the working directory here instead."},
				"timeout":    map[string]any{"type": "integer", "minimum": 0, "description": "Timeout in milliseconds. Set to 0 to disable the timeout. Defaults to 120000 for foreground commands. Background commands have no timeout by default."},
				"background": map[string]any{"type": "boolean", "description": "Run the command in the background and return immediately (useful for dev servers and long-running builds). You do not need to use '&' at the end of the command when using this parameter. You will be notified when it completes. DO NOT poll for completion."},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
	},
}

const fingerprintToolsNote = "Do not call the `read` or `shell` tools; they are placeholders that do nothing. Use the other tools you have been given instead."

// fingerprintToolName reads a tool name from either the chat shape ({function:{name}}) or the flat
// Responses shape ({name}).
func fingerprintToolName(tool map[string]any) string {
	if name, ok := tool["name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if function, ok := tool["function"].(map[string]any); ok {
		if name, ok := function["name"].(string); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// applyFingerprintTools appends the missing `read`/`shell` definitions in place and tells the model
// they are placeholders. It returns true when the body was changed.
func applyFingerprintTools(body map[string]json.RawMessage) bool {
	changed := false

	var tools []map[string]any
	if raw, ok := body["tools"]; ok {
		_ = json.Unmarshal(raw, &tools)
	}

	declared := map[string]bool{}
	for _, tool := range tools {
		if name := fingerprintToolName(tool); name != "" {
			declared[name] = true
		}
	}

	appended := false
	for name := range openCodeToolDefinitions {
		if declared[name] {
			continue
		}
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": openCodeToolDefinitions[name]["description"],
				"parameters":  openCodeToolDefinitions[name]["parameters"],
			},
		})
		appended = true
		changed = true
	}
	if appended {
		if encoded, err := json.Marshal(tools); err == nil {
			body["tools"] = encoded
		}
		// Chat-shaped bodies get the placeholder note as the first system message; bodies that carry
		// an explicit `system` field are left alone, like upstream.
		if _, hasSystem := body["system"]; !hasSystem {
			var messages []map[string]any
			if raw, ok := body["messages"]; ok {
				_ = json.Unmarshal(raw, &messages)
			}
			note := map[string]any{"role": "system", "content": fingerprintToolsNote}
			if len(messages) > 0 && messages[0]["role"] == "system" {
				if content, ok := messages[0]["content"].(string); ok {
					messages[0] = map[string]any{"role": "system", "content": content + "\n\n" + fingerprintToolsNote}
				}
			} else {
				messages = append([]map[string]any{note}, messages...)
			}
			if encoded, err := json.Marshal(messages); err == nil {
				body["messages"] = encoded
			}
		}
	}

	// Free-tier requests must stream; upstream answers a non-streaming free request with 403/429.
	if raw, ok := body["stream"]; !ok || string(raw) != "true" {
		body["stream"] = json.RawMessage("true")
		changed = true
	}
	return changed
}

// freeTierUpstreamBase rewrites the configured upstream base to the inference lane used by free models.
func freeTierUpstreamBase(baseURL string) string {
	if !strings.Contains(baseURL, openCodeZenBase) {
		return baseURL
	}
	return strings.Replace(baseURL, openCodeZenBase, openCodeInferenceBase, 1)
}
