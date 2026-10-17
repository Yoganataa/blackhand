package httpapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeProviderOverride(t *testing.T) {
	cases := []struct {
		name       string
		headers    map[string]any
		wantLength int
		wantError  string
	}{
		{"empty payload clears", map[string]any{}, 0, ""},
		{"valid header", map[string]any{"x-test": "1"}, 1, ""},
		{"blocked authorization", map[string]any{"Authorization": "Bearer x"}, 0, "Header Authorization cannot be overridden"},
		{"blocked host lowercase", map[string]any{"host": "evil"}, 0, "Header host cannot be overridden"},
		{"blocked content-type", map[string]any{"Content-Type": "text/plain"}, 0, "Header Content-Type cannot be overridden"},
		{"invalid name", map[string]any{"Bad Name": "1"}, 0, "Invalid header name: Bad Name"},
		{"crlf in value", map[string]any{"x-test": "a\r\nb"}, 0, "Invalid value for header x-test"},
		{"blank value ignored", map[string]any{"x-test": ""}, 0, ""},
		{"non string ignored", map[string]any{"x-test": nil}, 0, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			clean, message := normalizeProviderOverride(testCase.headers)
			if message != testCase.wantError {
				t.Fatalf("error = %q, want %q", message, testCase.wantError)
			}
			if len(clean) != testCase.wantLength {
				t.Fatalf("clean = %#v, want %d entries", clean, testCase.wantLength)
			}
		})
	}
}

func TestNormalizeProviderOverrideLimits(t *testing.T) {
	tooMany := map[string]any{}
	for index := 0; index <= maxOverrideHeaders; index++ {
		tooMany[fmt.Sprintf("x-header-%d", index)] = "value"
	}
	if _, message := normalizeProviderOverride(tooMany); !strings.HasPrefix(message, "Too many headers") {
		t.Fatalf("error = %q, want a too-many-headers message", message)
	}

	long := map[string]any{"x-long": strings.Repeat("a", maxOverrideValueBytes+1)}
	if _, message := normalizeProviderOverride(long); !strings.HasPrefix(message, "Header x-long value too long") {
		t.Fatalf("error = %q, want a value-too-long message", message)
	}

	atLimit := map[string]any{"x-ok": strings.Repeat("a", maxOverrideValueBytes)}
	if clean, message := normalizeProviderOverride(atLimit); message != "" || len(clean) != 1 {
		t.Fatalf("value at the limit should pass, got clean=%v error=%q", clean, message)
	}
}
