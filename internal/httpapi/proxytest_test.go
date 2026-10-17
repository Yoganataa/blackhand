package httpapi

import (
	"errors"
	"strings"
	"testing"
)

func TestSafeErrorRedactsCredentialMaterial(t *testing.T) {
	input := `proxy failed for http://alice:super-secret@proxy.example:8080/path?token=tok_123&api_key=key_456`
	got := safeError(errors.New(input))

	for _, secret := range []string{"alice:super-secret", "tok_123", "key_456"} {
		if strings.Contains(got, secret) {
			t.Fatalf("safeError leaked %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "http://[redacted]@proxy.example:8080/path?token=[redacted]&api_key=[redacted]") {
		t.Fatalf("credential URL was not safely redacted: %q", got)
	}
}
