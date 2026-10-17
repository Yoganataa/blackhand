package tokensaver

import (
	"context"
	"strings"
	"testing"
)

func TestCompressMessagesReducesBuildOutput(t *testing.T) {
	output:=strings.Repeat("Compiling package-example v1.2.3\n",60)+"Finished release [optimized] target(s) in 3.0s\n"
	body:=map[string]any{"messages":[]any{
		map[string]any{"role":"user","content":"build this"},
		map[string]any{"role":"tool","content":output},
	}}
	stats:=compressMessages(body)
	if len(stats.Hits)==0{t.Fatal("expected RTK filter to match build output")}
	messages:=body["messages"].([]any)
	compressed:=messages[1].(map[string]any)["content"].(string)
	if len(compressed)>=len(output){t.Fatalf("output did not shrink: %d >= %d",len(compressed),len(output))}
	if !strings.Contains(compressed,"Finished release"){t.Fatalf("lost build summary: %q",compressed)}
}
func TestShortAndErrorOutputRemainUnchanged(t *testing.T) {
	short:="short result"
	if got:=compressText(short,"test",&Stats{});got!=short{t.Fatalf("short text changed: %q",got)}
	errorBlock:="error[E0425]: cannot find value\n --> src/main.rs:12:3\n  |\n12 | missing\n  | ^^^^^^^\n"
	stats:=Stats{}
	if got:=compressText(errorBlock,"test",&stats);got!=errorBlock{t.Fatalf("error output should fail open, got %q",got)}
}
func TestPromptSaversOnlyChangeSystemPrompt(t *testing.T) {
	body:=map[string]any{"messages":[]any{map[string]any{"role":"user","content":"hello"}}}
	Process(New(),body,map[string]any{"rtkEnabled":false,"headroomEnabled":false,"cavemanEnabled":true,"cavemanLevel":"lite","ponytailEnabled":true,"ponytailLevel":"lite"},"oc/test")
	messages:=body["messages"].([]any)
	if len(messages)!=2{t.Fatalf("expected one injected system message, got %d messages",len(messages))}
	if messages[0].(map[string]any)["role"]!="system"{t.Fatal("system prompt not inserted first")}
	if messages[1].(map[string]any)["content"]!="hello"{t.Fatal("user content was mutated")}
}
func TestHeadroomFailureIsFailOpenAndCircuitBreaks(t *testing.T) {
	p:=New()
	body:=map[string]any{"messages":[]any{map[string]any{"role":"user","content":strings.Repeat("context ",100)}}}
	before:=body["messages"]
	_,reason:=p.headroom(context.Background(),body,map[string]any{"headroomUrl":"http://127.0.0.1:1","headroomTimeoutMs":100},"m")
	if reason==""{t.Fatal("expected failure reason")}
	if body["messages"]!=before{t.Fatal("Headroom failure mutated input")}
	started:=p.circuitUntil
	if started.IsZero(){t.Fatal("expected circuit breaker")}
	_,reason=p.headroom(context.Background(),body,map[string]any{"headroomUrl":"http://127.0.0.1:1"},"m")
	if !strings.Contains(reason,"circuit"){t.Fatalf("expected open circuit, got %q",reason)}
}
