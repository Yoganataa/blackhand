package cloudflared

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeFakeCloudflared creates an executable that prints the given lines then
// sleeps, mimicking a real cloudflared process long enough for the supervisor
// to observe readiness. The script is a .bat on Windows and a shell script
// elsewhere, so the test runs the same way on every platform CI builds on.
//
// The Windows sleep is `ping -n 31 127.0.0.1` rather than `timeout /t 30`:
// `timeout` fails immediately when stdin is redirected, which is exactly how a
// child process is started, so the fake would exit before the supervisor
// looked at it.
func writeFakeCloudflared(t *testing.T, dir string, lines []string) string {
	t.Helper()
	var path, body string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "cloudflared.bat")
		var b strings.Builder
		b.WriteString("@echo off\n")
		for _, line := range lines {
			b.WriteString("echo " + line + "\n")
		}
		b.WriteString("ping -n 31 127.0.0.1 >nul\n")
		body = b.String()
	} else {
		path = filepath.Join(dir, "cloudflared.sh")
		var b strings.Builder
		b.WriteString("#!/bin/sh\n")
		for _, line := range lines {
			b.WriteString("echo '" + line + "'\n")
		}
		b.WriteString("sleep 30\n")
		body = b.String()
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSupervisorOffModeDoesNothing(t *testing.T) {
	sup := New(Config{Mode: ModeOff})
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sup.Status().Running {
		t.Fatal("off mode must not spawn a process")
	}
	if sup.Status().URL != "" {
		t.Fatalf("unexpected URL %q", sup.Status().URL)
	}
}

func TestSupervisorNamedModeReadsConfigHostname(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeCloudflared(t, dir, []string{"Registered tunnel connection"})
	cfg := Config{
		Mode: ModeNamed,
		Bin:  bin,
		ConfigYAML: "tunnel: abc\n" +
			"credentials-file: /tmp/creds.json\n" +
			"ingress:\n" +
			"  - hostname: blackhand-test.example.com\n" +
			"    service: http://localhost:20130\n",
		WorkDir: dir,
	}
	sup := New(cfg)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sup.Stop()

	st := sup.Status()
	if !st.Running {
		t.Fatalf("not running; err=%q", st.LastError)
	}
	if st.URL != "https://blackhand-test.example.com" {
		t.Fatalf("URL = %q, want the ingress hostname", st.URL)
	}
	if st.PID <= 0 {
		t.Fatalf("PID not recorded: %d", st.PID)
	}
}

func TestSupervisorQuickModeCapturesUrlFromLog(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeCloudflared(t, dir, []string{"INF Registered tunnel connection", "https://foo-quick.trycloudflare.com"})
	cfg := Config{
		Mode:       ModeQuick,
		Bin:        bin,
		OriginURL:  "http://127.0.0.1:20130",
		WorkDir:    dir,
	}
	sup := New(cfg)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sup.Stop()

	if got := sup.Status().URL; got != "https://foo-quick.trycloudflare.com" {
		t.Fatalf("URL = %q", got)
	}
}

func TestSupervisorReadinessTimeout(t *testing.T) {
	dir := t.TempDir()
	// A process that never prints the readiness line.
	var path, body string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "cloudflared.bat")
		body = "@echo off\nping -n 31 127.0.0.1 >nul\n"
	} else {
		path = filepath.Join(dir, "cloudflared.sh")
		body = "#!/bin/sh\nsleep 30\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Mode:             ModeNamed,
		Bin:              path,
		ConfigYAML:       "ingress:\n  - hostname: x.example.com\n    service: http://localhost\n",
		WorkDir:          dir,
		ReadinessTimeout: 200 * time.Millisecond,
	}
	sup := New(cfg)
	if err := sup.Start(context.Background()); err == nil {
		t.Fatal("Start should fail when cloudflared never becomes ready")
	}
	st := sup.Status()
	if st.Running {
		t.Fatal("supervisor must be stopped after a readiness timeout")
	}
	if !strings.Contains(st.LastError, "did not become ready") {
		t.Fatalf("LastError = %q", st.LastError)
	}
}

func TestSupervisorNamedModeRequiresConfig(t *testing.T) {
	sup := New(Config{Mode: ModeNamed, WorkDir: t.TempDir()})
	if err := sup.Start(context.Background()); err == nil {
		t.Fatal("Start without config should fail")
	}
}

func TestSupervisorQuickModeRequiresOrigin(t *testing.T) {
	sup := New(Config{Mode: ModeQuick, WorkDir: t.TempDir()})
	if err := sup.Start(context.Background()); err == nil {
		t.Fatal("Start without origin URL should fail")
	}
}

// writeCountingFakeCloudflared writes a fake that appends a line to counter
// on every start, so a test can tell how many times the supervisor actually
// launched it. When exitAfter is non-empty the fake exits right after
// announcing readiness, which is how a dropped tunnel looks.
func writeCountingFakeCloudflared(t *testing.T, dir string, exitAfter bool) (bin, counter string) {
	t.Helper()
	counter = filepath.Join(dir, "starts.txt")
	appendLine := "echo x>> \"" + counter + "\""
	if runtime.GOOS == "windows" {
		bin = filepath.Join(dir, "cloudflared.bat")
		body := "@echo off\n" + appendLine + "\necho Registered tunnel connection\n"
		if exitAfter {
			body += "exit /b 0\n"
		} else {
			body += "ping -n 31 127.0.0.1 >nul\n"
		}
		if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return bin, counter
	}
	bin = filepath.Join(dir, "cloudflared.sh")
	body := "#!/bin/sh\n" + appendLine + "\necho 'Registered tunnel connection'\n"
	if exitAfter {
		body += "exit 0\n"
	} else {
		body += "sleep 30\n"
	}
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, counter
}

// countStarts reports how many times the fake was launched.
func countStarts(t *testing.T, counter string) int {
	t.Helper()
	data, err := os.ReadFile(counter)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return len(strings.Fields(string(data)))
}

func TestSupervisorRestartsAfterUnexpectedExit(t *testing.T) {
	dir := t.TempDir()
	bin, counter := writeCountingFakeCloudflared(t, dir, true)
	sup := New(Config{
		Mode: ModeNamed,
		Bin:  bin,
		ConfigYAML: "ingress:\n  - hostname: blackhand.example.com\n    service: http://localhost\n",
		WorkDir:    dir,
	})
	defer sup.Stop()

	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The child exits on its own. The supervisor must relaunch it instead of
	// leaving the tunnel down until an operator notices.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if countStarts(t, counter) > 1 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("supervisor did not restart the tunnel; starts = %d", countStarts(t, counter))
}

func TestSupervisorStopSuppressesRestart(t *testing.T) {
	dir := t.TempDir()
	// This fake stays alive, so the only way a second start can happen is the
	// supervisor's own restart logic.
	bin, counter := writeCountingFakeCloudflared(t, dir, false)
	sup := New(Config{
		Mode: ModeNamed,
		Bin:  bin,
		ConfigYAML: "ingress:\n  - hostname: blackhand.example.com\n    service: http://localhost\n",
		WorkDir:    dir,
	})

	if err := sup.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := countStarts(t, counter); got != 1 {
		t.Fatalf("starts = %d, want 1", got)
	}

	// Stop cancels the child, which makes the reaper observe an exit. The
	// supervisor must not relaunch a tunnel that was deliberately stopped.
	sup.Stop()

	time.Sleep(3 * time.Second)
	if got := countStarts(t, counter); got != 1 {
		t.Fatalf("Stop must suppress the automatic restart, starts = %d", got)
	}
	if sup.Status().Running {
		t.Fatal("supervisor must not be running after Stop")
	}
}

func TestURLFromConfig(t *testing.T) {
	cases := []struct {
		yaml string
		want string
	}{
		{"ingress:\n  - hostname: example.com\n    service: http://localhost\n", "https://example.com"},
		{"ingress:\n  - hostname: \"example.com\"\n    service: http://localhost\n", "https://example.com"},
		{"ingress:\n  - service: http://localhost\n", ""},
		{"", ""},
	}
	for _, testCase := range cases {
		if got := URLFromConfig(testCase.yaml); got != testCase.want {
			t.Fatalf("URLFromConfig(%q) = %q, want %q", testCase.yaml, got, testCase.want)
		}
	}
}

func TestURLFromLogLine(t *testing.T) {
	if got := URLFromLogLine("foo https://abc-123.trycloudflare.com bar"); got != "https://abc-123.trycloudflare.com" {
		t.Fatalf("got %q", got)
	}
	if got := URLFromLogLine("no url here"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveFilesMaterializesInlineInputs(t *testing.T) {
	dir := t.TempDir()
	sup := New(Config{
		Mode:            ModeNamed,
		WorkDir:         dir,
		ConfigYAML:      "ingress:\n  - hostname: x.example.com\n    service: http://localhost\n",
		Credentials:     `{"AccountTag":"tag","TunnelSecret":"secret","TunnelID":"id"}`,
	})
	configPath, credentialsPath, err := sup.ResolveFiles()
	if err != nil {
		t.Fatal(err)
	}
	if configPath != filepath.Join(dir, "config.yml") {
		t.Fatalf("config path = %q", configPath)
	}
	if credentialsPath != filepath.Join(dir, "credentials.json") {
		t.Fatalf("credentials path = %q", credentialsPath)
	}
	for _, path := range []string{configPath, credentialsPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		// Unix permission bits are not meaningful on Windows (the OS reports 0666 for
		// everything); the 0600 mode is still passed to WriteFile for Unix systems.
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v, want 0600", path, info.Mode().Perm())
		}
	}
}

func TestResolveFilesPrefersExplicitPaths(t *testing.T) {
	dir := t.TempDir()
	sup := New(Config{
		Mode:            ModeNamed,
		WorkDir:         dir,
		ConfigFile:      "/etc/cloudflared/config.yml",
		CredentialsFile: "/etc/cloudflared/creds.json",
		ConfigYAML:      "should be ignored",
		Credentials:     "should be ignored",
	})
	configPath, credentialsPath, err := sup.ResolveFiles()
	if err != nil {
		t.Fatal(err)
	}
	if configPath != "/etc/cloudflared/config.yml" || credentialsPath != "/etc/cloudflared/creds.json" {
		t.Fatalf("explicit paths not honored: %q %q", configPath, credentialsPath)
	}
}
