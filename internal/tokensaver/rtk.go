package tokensaver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	minCompressBytes = 500
	maxRawBytes = 10 << 20
)

type Hit struct {
	Shape string
	Filter string
	Saved int
}
type Stats struct {
	BytesBefore int
	BytesAfter int
	Hits []Hit
	Headroom string
	PXPipe string
}
type Processor struct {
	client *http.Client
	mu sync.Mutex
	circuitUntil time.Time
}

func New() *Processor {
	return &Processor{client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *Processor) Process(ctx context.Context, body map[string]any, settings map[string]any, model string) Stats {
	stats := Stats{Hits: []Hit{}}
	if enabled(settings["rtkEnabled"], true) {
		stats = compressMessages(body)
	}
	if enabled(settings["headroomEnabled"], true) {
		result, status := p.compressHeadroom(ctx, body, settings, model)
		stats.Headroom = status
		if result != nil {
			if messages, ok := result["messages"].([]any); ok {
				body["messages"] = messages
			}
		}
	}
	if enabled(settings["cavemanEnabled"], false) {
		injectSystemPrompt(body, cavemanPrompt(stringValue(settings, "cavemanLevel", "full")))
	}
	if enabled(settings["ponytailEnabled"], true) {
		injectSystemPrompt(body, ponytailPrompt(stringValue(settings, "ponytailLevel", "full")))
	}
	if enabled(settings["pxpipeEnabled"], false) {
		// The upstream implementation can invoke an external PXPipe helper.
		// This portable build does not install helper binaries implicitly.
		stats.PXPipe = "skipped: PXPipe adapter is not bundled; request preserved"
	}
	return stats
}

func enabled(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return fallback
	}
}
func stringValue(settings map[string]any, key, fallback string) string {
	if v, ok := settings[key].(string); ok && v != "" { return v }
	return fallback
}
func intValue(settings map[string]any, key string, fallback int) int {
	switch v := settings[key].(type) {
	case float64:
		if v > 0 { return int(v) }
	case int:
		if v > 0 { return v }
	}
	return fallback
}

func (p *Processor) compressHeadroom(ctx context.Context, body map[string]any, settings map[string]any, model string) (map[string]any, string) {
	messages, ok := body["messages"].([]any)
	if !ok {
		return nil, "skipped: request shape is not OpenAI messages[]"
	}
	p.mu.Lock()
	if time.Now().Before(p.circuitUntil) {
		p.mu.Unlock()
		return nil, "skipped: circuit open after recent Headroom failure"
	}
	p.mu.Unlock()

	timeout := intValue(settings, "headroomTimeoutMs", 10000)
	if timeout > 32000 { timeout = 32000 }
	payload := map[string]any{"messages": messages, "model": model}
	if enabled(settings["headroomCompressUserMessages"], true) {
		payload["config"] = map[string]any{"compress_user_messages": true}
	}
	raw, err := json.Marshal(payload)
	if err != nil { return nil, "skipped: could not encode request" }
	if len(raw) > 1000000 && timeout < 32000 { timeout = 32000 } else if len(raw) > 250000 && timeout < 15000 { timeout = 15000 }

	endpoint := strings.TrimRight(stringValue(settings, "headroomUrl", "http://localhost:8787"), "/")
	if !strings.HasSuffix(endpoint, "/v1/compress") { endpoint += "/v1/compress" }
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil { return nil, "skipped: invalid Headroom URL" }
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		p.openCircuit()
		return nil, "skipped: Headroom unavailable; fail-open"
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		p.openCircuit()
		return nil, fmt.Sprintf("skipped: Headroom HTTP %d; fail-open", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&result); err != nil {
		return nil, "skipped: invalid Headroom response"
	}
	if _, ok := result["messages"].([]any); !ok { return nil, "skipped: response has no messages[]" }
	p.mu.Lock()
	p.circuitUntil = time.Time{}
	p.mu.Unlock()
	return result, "applied"
}
func (p *Processor) openCircuit() {
	p.mu.Lock()
	p.circuitUntil = time.Now().Add(30 * time.Second)
	p.mu.Unlock()
}

func injectSystemPrompt(body map[string]any, prompt string) {
	if strings.TrimSpace(prompt) == "" { return }
	if existing, ok := body["system"].(string); ok {
		body["system"] = existing + "\n\n" + prompt
		return
	}
	messages, ok := body["messages"].([]any)
	if !ok { return }
	for _, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok || msg["role"] != "system" { continue }
		if current, ok := msg["content"].(string); ok {
			msg["content"] = current + "\n\n" + prompt
			return
		}
	}
	body["messages"] = append([]any{map[string]any{"role":"system", "content":prompt}}, messages...)
}
func cavemanPrompt(level string) string {
	switch strings.ToLower(level) {
	case "lite":
		return "Use concise, clear prose. Remove filler but preserve technical detail, code, paths, commands, URLs, errors and ordered steps. Keep safety-critical instructions unambiguous."
	case "ultra":
		return "Use very terse prose for ordinary explanations. Keep technical identifiers exact; describe security warnings and irreversible actions clearly."
	case "wenyan", "wenyan-lite", "wenyan-ultra":
		return "Prefer compact classical-Chinese-style prose for noncritical explanations. Preserve code and technical identifiers verbatim; use clear language for safety-critical directions."
	default:
		return "Use terse, direct prose without filler. Preserve technical meaning, code, paths, commands, URLs, errors and safety warnings."
	}
}
func ponytailPrompt(level string) string {
	switch strings.ToLower(level) {
	case "ultra":
		return "Apply YAGNI rigorously. Prefer deletion, standard library and native features. Keep the smallest correct implementation; never omit validation, error handling, security, accessibility or requested behavior."
	case "lite":
		return "Prefer the simplest correct implementation. Avoid unnecessary abstractions, dependencies and files. Never skip trust-boundary validation or essential error handling."
	default:
		return "Prefer the simplest correct implementation, standard library and native features. Avoid unrequested abstractions and dependencies without simplifying away required behavior."
	}
}

func compressMessages(body map[string]any) Stats {
	stats := Stats{Hits: []Hit{}}
	items, ok := body["messages"].([]any)
	if !ok { items, ok = body["input"].([]any) }
	if !ok { return stats }
	for _, raw := range items {
		msg, ok := raw.(map[string]any)
		if !ok { continue }
		if msg["type"] == "function_call_output" {
			if text, ok := msg["output"].(string); ok { msg["output"] = compressText(text, "openai-responses-string", &stats) }
			continue
		}
		if msg["role"] == "tool" {
			if text, ok := msg["content"].(string); ok { msg["content"] = compressText(text, "openai-tool", &stats); continue }
		}
		parts, ok := msg["content"].([]any)
		if !ok { continue }
		if msg["role"] == "tool" {
			for _, part := range parts {
				block, ok := part.(map[string]any)
				if !ok || block["type"] != "text" { continue }
				if text, ok := block["text"].(string); ok { block["text"] = compressText(text, "openai-tool-array", &stats) }
			}
			continue
		}
		for _, part := range parts {
			block, ok := part.(map[string]any)
			if !ok || block["type"] != "tool_result" || block["is_error"] == true { continue }
			if text, ok := block["content"].(string); ok {
				block["content"] = compressText(text, "claude-string", &stats)
				continue
			}
			children, ok := block["content"].([]any)
			if !ok { continue }
			for _, child := range children {
				textBlock, ok := child.(map[string]any)
				if !ok || textBlock["type"] != "text" { continue }
				if text, ok := textBlock["text"].(string); ok { textBlock["text"] = compressText(text, "claude-array", &stats) }
			}
		}
	}
	return stats
}

func compressText(input, shape string, stats *Stats) string {
	before := len([]byte(input))
	stats.BytesBefore += before
	if before < minCompressBytes || before > maxRawBytes { stats.BytesAfter += before; return input }
	filter := detect(input)
	if filter == "" { stats.BytesAfter += before; return input }
	output := applyFilter(filter, input)
	after := len([]byte(output))
	if output == "" || after >= before { stats.BytesAfter += before; return input }
	stats.BytesAfter += after
	stats.Hits = append(stats.Hits, Hit{Shape:shape, Filter:filter, Saved:before-after})
	return output
}

func detect(input string) string {
	head := input
	if len(head) > 1024 { head = head[:1024] }
	switch {
	case strings.Contains(head, "diff --git ") || strings.Contains(head, "@@ "):
		return "git-diff"
	case strings.Contains(head, "On branch ") || strings.Contains(head, "nothing to commit") || strings.Contains(head, "Untracked files:"):
		return "git-status"
	case strings.Contains(head, "commit ") && strings.Contains(head, "Author:") && strings.Contains(head, "Date:"):
		return "git-log"
	case strings.Contains(strings.ToLower(head), "compiling ") || strings.Contains(strings.ToLower(head), "downloading ") || strings.Contains(strings.ToLower(head), "build failed") || strings.Contains(strings.ToLower(head), "build success"):
		return "build-output"
	case looksLikeGrep(head):
		return "grep"
	case looksLikeTree(head):
		return "tree"
	case looksLikeLongListing(head):
		return "ls"
	case allLikelyPaths(head):
		return "find"
	case looksLikeNumberedFile(input):
		return "read-numbered"
	case len(strings.Split(input, "\n")) >= 250:
		return "smart-truncate"
	case hasDuplicateRuns(input):
		return "dedup-log"
	default:
		return ""
	}
}

func applyFilter(name, input string) string {
	switch name {
	case "git-diff": return filterGitDiff(input)
	case "git-status": return filterGitStatus(input)
	case "git-log": return filterGitLog(input)
	case "build-output": return filterBuildOutput(input)
	case "grep": return filterGrep(input)
	case "tree": return filterTree(input)
	case "ls": return filterLS(input)
	case "find": return filterFind(input)
	case "read-numbered": return filterNumbered(input)
	case "smart-truncate": return filterSmartTruncate(input)
	case "dedup-log": return filterDedup(input)
	default: return input
	}
}

func filterGitDiff(input string) string {
	lines := strings.Split(input, "\n")
	out := []string{}
	file := ""
	added, removed, kept := 0, 0, 0
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if file != "" { out = append(out, fmt.Sprintf("  +%d -%d", added, removed)) }
			parts := strings.Split(line, " b/")
			file = line
			if len(parts) > 1 { file = parts[len(parts)-1] }
			out = append(out, "\n"+file)
			added, removed, kept = 0, 0, 0
			continue
		}
		if strings.HasPrefix(line, "@@") { out = append(out, "  "+line); kept = 0; continue }
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") { continue }
		if strings.HasPrefix(line, "+") { added++; if kept < 24 { out = append(out, "  "+line); kept++ }; continue }
		if strings.HasPrefix(line, "-") { removed++; if kept < 24 { out = append(out, "  "+line); kept++ }; continue }
		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "new file mode") || strings.HasPrefix(line, "deleted file mode") { continue }
		if kept < 24 && strings.TrimSpace(line) != "" { out = append(out, "  "+line); kept++ }
		if len(out) >= 500 { out = append(out, "... remaining diff omitted"); break }
	}
	if file != "" { out = append(out, fmt.Sprintf("  +%d -%d", added, removed)) }
	if len(out) == 0 { return input }
	out = append(out, "[compact diff; full diff remains available from the original tool]")
	return strings.Join(out, "\n")
}
func filterGitStatus(input string) string {
	out := []string{}
	for _, line := range strings.Split(input, "\n") {
		t := strings.TrimSpace(line)
		if t == "" { continue }
		if strings.HasPrefix(t, "On branch ") || strings.HasPrefix(t, "##") || strings.HasPrefix(t, "Changes ") ||
			strings.HasPrefix(t, "Untracked files:") || strings.HasPrefix(t, "nothing to commit") ||
			strings.HasPrefix(t, "modified:") || strings.HasPrefix(t, "new file:") ||
			strings.HasPrefix(t, "deleted:") || strings.HasPrefix(t, "renamed:") ||
			strings.HasPrefix(t, "both modified:") {
			out = append(out, line)
		}
		if len(out) >= 80 { out = append(out, "... remaining status lines omitted"); break }
	}
	if len(out) == 0 { return input }
	return strings.Join(out, "\n")
}
func filterGitLog(input string) string {
	out := []string{}
	inCommit := false
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "commit ") {
			out = append(out, t)
			inCommit = true
		} else if !inCommit {
			out = append(out, line)
		} else if strings.HasPrefix(t, "Author:") || strings.HasPrefix(t, "Date:") {
			out = append(out, t)
		} else if t != "" && !strings.HasPrefix(t, "|") && !strings.HasPrefix(t, "\\") {
			out = append(out, "  "+t)
		}
		if len(out) >= 200 { out = append(out, "... remaining log entries omitted"); break }
	}
	if len(out) >= len(lines) { return input }
	return strings.Join(out, "\n")
}
var (
	cargoErrorContinuation = regexp.MustCompile(`^\\s*(-->|\\||\\d+\\s*\\||=)`)
	buildCountSummary = regexp.MustCompile(`^(added|removed|changed|audited|installed)\\s+\\d+\\s+package`)
	buildSimpleSummary = regexp.MustCompile(`^\\d+\\s+(vulnerabilities|packages?|warnings?|errors?)`)
	buildAddressSummary = regexp.MustCompile(`^To address .* issues`)
	buildNpmCommandSummary = regexp.MustCompile("` + "`" + `^Run \\`npm (audit|fund)\\`` + "`" + `)
)

func filterBuildOutput(input string) string {
	// Ported from Yoganataa/9router open-sse/rtk/filters/buildOutput.js
	// at db6112897399ac981ca73ed573d14102e84ca0f1. Keep the grouping/order
	// stable so the fixture tests protect byte-for-byte compatibility here.
	errorsOut := []string{}
	warnings := []string{}
	deprecations := []string{}
	summary := []string{}
	compilingCount, downloadingCount := 0, 0
	inCargoError := false

	for _, line := range strings.Split(input, "\\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if inCargoError {
			if trimmed == "" {
				inCargoError = false
				continue
			}
			if cargoErrorContinuation.MatchString(line) {
				errorsOut = append(errorsOut, line)
				continue
			}
			inCargoError = false
		}
		if trimmed == "" {
			continue
		}

		switch {
		case strings.HasPrefix(lower, "npm err!") || strings.HasPrefix(lower, "npm error") || strings.HasPrefix(lower, "yarn error"):
			errorsOut = append(errorsOut, line)
			continue
		case strings.HasPrefix(lower, "npm warn deprecated"):
			deprecations = append(deprecations, line)
			continue
		case strings.HasPrefix(lower, "npm warn") || strings.HasPrefix(lower, "yarn warn"):
			warnings = append(warnings, line)
			continue
		case strings.HasPrefix(lower, "error[") || strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "error -->"):
			errorsOut = append(errorsOut, line)
			inCargoError = true
			continue
		case strings.HasPrefix(lower, "warning[") || strings.HasPrefix(lower, "warning:") || strings.HasPrefix(lower, "warning -->"):
			warnings = append(warnings, line)
			inCargoError = true
			continue
		case strings.HasPrefix(lower, "error:"):
			errorsOut = append(errorsOut, line)
			continue
		case strings.HasPrefix(lower, "[error]") || strings.HasPrefix(lower, "build failed"):
			errorsOut = append(errorsOut, line)
			continue
		case strings.HasPrefix(lower, "[warning]"):
			warnings = append(warnings, line)
			continue
		case strings.HasPrefix(lower, "compiling ") && hasBuildTokenAfterPrefix(trimmed, "compiling "):
			compilingCount++
			continue
		case strings.HasPrefix(lower, "downloading ") && hasBuildTokenAfterPrefix(trimmed, "downloading "):
			downloadingCount++
			continue
		case strings.HasPrefix(lower, "fetching "):
			downloadingCount++
			continue
		case buildCountSummary.MatchString(lower) || strings.HasPrefix(lower, "finished ") ||
			strings.HasPrefix(lower, "build success") || strings.HasPrefix(lower, "successfully installed") ||
			strings.HasPrefix(lower, "successfully built") || buildSimpleSummary.MatchString(lower) ||
			buildAddressSummary.MatchString(trimmed) || buildNpmCommandSummary.MatchString(trimmed) ||
			strings.Contains(lower, "packages are looking for funding"):
			summary = append(summary, line)
			continue
		}
	}

	out := []string{}
	for i, line := range deprecations {
		if i >= 3 {
			break
		}
		out = append(out, line)
	}
	if len(deprecations) > 3 {
		out = append(out, fmt.Sprintf("... +%d more deprecated packages", len(deprecations)-3))
	}
	if compilingCount > 0 {
		out = append(out, fmt.Sprintf("Compiled %d packages", compilingCount))
	}
	if downloadingCount > 0 {
		out = append(out, fmt.Sprintf("Downloaded %d packages", downloadingCount))
	}
	out = append(out, errorsOut...)
	for i, line := range warnings {
		if i >= 5 {
			break
		}
		out = append(out, line)
	}
	if len(warnings) > 5 {
		out = append(out, fmt.Sprintf("... +%d more warnings", len(warnings)-5))
	}
	out = append(out, summary...)
	if len(out) == 0 {
		return input
	}
	return strings.Join(out, "\\n")
}

func hasBuildTokenAfterPrefix(line, prefix string) bool {
	return len(strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), prefix))) > 0
}

func looksLikeGrep(input string) bool {
	lines := strings.Split(strings.TrimSpace(input), "\n")
	if len(lines) < 3 { return false }
	matches := 0
	for _, line := range lines { parts := strings.SplitN(line, ":", 3); if len(parts) >= 3 && len(parts[1]) > 0 && allDigits(parts[1]) { matches++ } }
	return matches >= 3
}
func allDigits(s string) bool { for _, r := range s { if r < '0' || r > '9' { return false } }; return s != "" }
func filterGrep(input string) string {
	out := []string{}
	counts := map[string]int{}
	for _, line := range strings.Split(input, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || !allDigits(parts[1]) { continue }
		counts[parts[0]]++
		if counts[parts[0]] <= 5 { out = append(out, parts[0]+":"+parts[1]+":"+strings.TrimSpace(parts[2])) }
	}
	total := 0
	for _, count := range counts { total += count }
	if total == 0 { return input }
	out = append([]string{fmt.Sprintf("%d matches in %d files", total, len(counts))}, out...)
	return strings.Join(out, "\n")
}
func looksLikeTree(input string) bool { return strings.Contains(input, "├──") || strings.Contains(input, "└──") || strings.Contains(input, "│  ") }
func filterTree(input string) string {
	out := []string{}
	for _, line := range strings.Split(input, "\n") {
		if strings.Contains(line, "├──") || strings.Contains(line, "└──") || strings.Contains(line, "│") || strings.HasSuffix(strings.TrimSpace(line), "/") {
			out = append(out, line)
		}
		if len(out) >= 200 { out = append(out, "... remaining tree entries omitted"); break }
	}
	if len(out) == 0 { return input }
	return strings.Join(out, "\n")
}
func looksLikeLongListing(input string) bool {
	count := 0
	for _, line := range strings.Split(input, "\n") { if len(line) > 10 && (strings.HasPrefix(line, "-rw") || strings.HasPrefix(line, "drw") || strings.HasPrefix(line, "lrw")) { count++ } }
	return count >= 3
}
func filterLS(input string) string {
	lines := strings.Split(input, "\n")
	out := []string{}
	exts := map[string]int{}
	kept := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "total ") { continue }
		fields := strings.Fields(t)
		name := fields[len(fields)-1]
		if name == "." || name == ".." || name == "node_modules" || name == ".git" || name == "target" || name == "dist" || name == "build" { continue }
		if i := strings.LastIndex(name, "."); i > 0 { exts[name[i:]]++ } else { exts["no ext"]++ }
		if kept < 80 { out = append(out, line); kept++ }
	}
	if len(out) == 0 || len(out) >= len(lines) { return input }
	sortKeys := []string{}
	for ext := range exts { sortKeys = append(sortKeys, ext) }
	// Avoid importing sort for the normal request hot path if no summary is needed.
	if len(sortKeys) > 0 { out = append(out, fmt.Sprintf("Summary: %d files, %d extensions", len(out), len(sortKeys))) }
	return strings.Join(out, "\n")
}
func allLikelyPaths(input string) bool {
	lines := strings.Split(strings.TrimSpace(input), "\n")
	if len(lines) < 3 { return false }
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.ContainsAny(t, " \t") || strings.Contains(t, ":") { return false }
	}
	return true
}
func filterFind(input string) string {
	lines := strings.Split(input, "\n")
	out := []string{}
	dirs := map[string]int{}
	for _, line := range lines {
		p := strings.TrimSpace(line)
		if p == "" { continue }
		i := strings.LastIndex(p, "/")
		dir := "."
		if i >= 0 { dir = p[:i] }
		dirs[dir]++
		if len(out) < 100 { out = append(out, p) }
	}
	if len(out) == 0 || len(out) >= len(lines) { return input }
	out = append([]string{fmt.Sprintf("%d paths", len(lines))}, out...)
	return strings.Join(out, "\n")
}
func looksLikeNumberedFile(input string) bool {
	lines := strings.Split(input, "\n")
	if len(lines) < 250 { return false }
	numbered := 0
	for _, line := range lines { if i := strings.Index(line, "|"); i > 0 && allDigits(strings.TrimSpace(line[:i])) { numbered++ } }
	return float64(numbered)/float64(len(lines)) >= 0.7
}
func filterNumbered(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) < 250 { return input }
	out := append([]string{}, lines[:100]...)
	out = append(out, fmt.Sprintf("... %d lines omitted", len(lines)-160))
	out = append(out, lines[len(lines)-60:]...)
	return strings.Join(out, "\n")
}
func filterSmartTruncate(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) < 250 { return input }
	out := append([]string{}, lines[:120]...)
	out = append(out, fmt.Sprintf("... %d lines omitted", len(lines)-180))
	out = append(out, lines[len(lines)-60:]...)
	return strings.Join(out, "\n")
}
func hasDuplicateRuns(input string) bool {
	lines := strings.Split(input, "\n")
	run := 0
	for i := 1; i < len(lines); i++ { if lines[i] != "" && lines[i] == lines[i-1] { run++; if run >= 2 { return true } } else { run = 0 } }
	return false
}
func filterDedup(input string) string {
	lines := strings.Split(input, "\n")
	out := []string{}
	previous := ""
	duplicates := 0
	blankSeen := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if !blankSeen { out = append(out, line) }
			blankSeen = true
			if duplicates > 0 { out = append(out, fmt.Sprintf("  ... %d duplicate lines", duplicates)) }
			previous = ""; duplicates = 0
			continue
		}
		blankSeen = false
		if line == previous { duplicates++; continue }
		if duplicates > 0 { out = append(out, fmt.Sprintf("  ... %d duplicate lines", duplicates)) }
		out = append(out, line)
		previous = line; duplicates = 0
		if len(out) >= 2000 { out = append(out, "... output truncated"); break }
	}
	if duplicates > 0 { out = append(out, fmt.Sprintf("  ... %d duplicate lines", duplicates)) }
	return strings.Join(out, "\n")
}
