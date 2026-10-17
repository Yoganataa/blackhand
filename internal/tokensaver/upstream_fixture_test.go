package tokensaver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOutputMatchesPinnedUpstreamFixture(t *testing.T) {
	// Expected bytes were transcribed from the behavior in:
	// Yoganataa/9router/open-sse/rtk/filters/buildOutput.js
	// at commit db6112897399ac981ca73ed573d14102e84ca0f1.
	// The fixture contains only synthetic output and no real project data.
	input, err := os.ReadFile(filepath.Join("testdata", "upstream-build-output.input.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("testdata", "upstream-build-output.expected.txt"))
	if err != nil {
		t.Fatal(err)
	}

	// Git may check text fixtures out as CRLF on Windows. Canonicalize line
	// endings before the byte-exact comparison so generated summary lines and
	// preserved input lines use one stable fixture representation on every OS.
	input = []byte(strings.ReplaceAll(strings.ReplaceAll(string(input), "\r\n", "\n"), "\r", "\n"))
	expected = []byte(strings.ReplaceAll(strings.ReplaceAll(string(expected), "\r\n", "\n"), "\r", "\n"))

	got := filterBuildOutput(string(input))
	if got != string(expected) {
		t.Fatalf("upstream build-output fixture mismatch\n--- got ---\n%s\n--- want ---\n%s", got, expected)
	}

	stats := Stats{}
	pipeline := compressText(string(input), "upstream-fixture", &stats)
	if pipeline != string(expected) {
		t.Fatalf("RTK pipeline diverged from fixture output")
	}
	if len(stats.Hits) != 1 || stats.Hits[0].Filter != "build-output" {
		t.Fatalf("expected build-output detection hit, got %#v", stats.Hits)
	}
	if stats.BytesAfter >= stats.BytesBefore {
		t.Fatalf("fixture should compress: before=%d after=%d", stats.BytesBefore, stats.BytesAfter)
	}
}
