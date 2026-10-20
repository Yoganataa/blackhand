package cloudflared

// Package cloudflared supervises a `cloudflared` process so a named Cloudflare Tunnel
// (credentials JSON + config YAML, supplied through env) can expose the gateway on a stable
// hostname instead of the random trycloudflare.com URL a quick tunnel produces.
//
// The supervisor is deliberately a thin process manager: it resolves its input files, spawns
// cloudflared, waits for the readiness line, and restarts with bounded backoff. Everything
// dashboard-facing (status, tunnelUrl, tunnelEnabled, the tunnel gate) is handled by the
// existing settings store and handlers, so this package has no dependency on them.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	ModeOff    = "off"
	ModeQuick  = "quick"
	ModeNamed  = "named"
	readiness   = 30 * time.Second
	maxRestarts = 5
	// stableAfter is how long a tunnel has to stay up before a drop counts as
	// a fresh failure instead of a continuation of the previous one.
	stableAfter = 60 * time.Second
)

// Config controls how the supervisor starts cloudflared. Inline material (JSON/YAML) is written
// to WorkDir with mode 0600 before cloudflared is started; file paths are used as-is.
type Config struct {
	Mode string

	Bin string // cloudflared executable; defaults to "cloudflared"

	// Named tunnel inputs.
	Name             string
	ConfigYAML       string // inline YAML
	ConfigFile       string // path to a YAML file
	Credentials      string // inline credentials JSON
	CredentialsFile  string // path to a credentials JSON file

	// Quick tunnel inputs.
	OriginURL string // local URL cloudflared proxies, e.g. http://127.0.0.1:20130

	// Explicit public URL, overrides anything parsed from the config or logs.
	URL string

	WorkDir string

	ReadinessTimeout time.Duration
}

// Status is the supervisor's observable state.
type Status struct {
	Mode      string
	Running   bool
	URL       string
	PID       int
	StartedAt time.Time
	LastError string
	Restarts  int
}

type Supervisor struct {
	cfg Config

	mu       sync.Mutex
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	pid      int
	url      string
	err      string
	start    time.Time
	restarts int
	running  bool
	// stopped distinguishes an operator-requested stop from a process that
	// died on its own. Without it the auto-restart would resurrect a tunnel
	// the user just disabled.
	stopped bool
	ready   chan struct{}
	close   chan struct{}
}

func New(cfg Config) *Supervisor {
	if cfg.Bin == "" {
		cfg.Bin = "cloudflared"
	}
	if cfg.ReadinessTimeout <= 0 {
		cfg.ReadinessTimeout = readiness
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.TempDir()
	}
	return &Supervisor{cfg: cfg, ready: make(chan struct{}, 1), close: make(chan struct{})}
}

// ResolveFiles materializes inline config/credentials into WorkDir and returns the file paths
// cloudflared should be pointed at. It does not touch the filesystem if both are already files.
func (s *Supervisor) ResolveFiles() (configPath, credentialsPath string, err error) {
	if err := os.MkdirAll(s.cfg.WorkDir, 0o755); err != nil {
		return "", "", err
	}

	// Credentials: prefer an explicit file, then materialize inline JSON.
	if strings.TrimSpace(s.cfg.CredentialsFile) != "" {
		credentialsPath = s.cfg.CredentialsFile
	} else if strings.TrimSpace(s.cfg.Credentials) != "" {
		credentialsPath = filepath.Join(s.cfg.WorkDir, "credentials.json")
		if err := os.WriteFile(credentialsPath, []byte(s.cfg.Credentials), 0o600); err != nil {
			return "", "", err
		}
	}

	// Config YAML: prefer an explicit file, then materialize inline YAML.
	if strings.TrimSpace(s.cfg.ConfigFile) != "" {
		configPath = s.cfg.ConfigFile
	} else if strings.TrimSpace(s.cfg.ConfigYAML) != "" {
		configPath = filepath.Join(s.cfg.WorkDir, "config.yml")
		if err := os.WriteFile(configPath, []byte(s.cfg.ConfigYAML), 0o600); err != nil {
			return "", "", err
		}
	}
	return configPath, credentialsPath, nil
}

// buildArgs returns the cloudflared argument list for the configured mode.
func (s *Supervisor) buildArgs(configPath string) ([]string, error) {
	switch s.cfg.Mode {
	case ModeNamed:
		if strings.TrimSpace(configPath) == "" {
			return nil, fmt.Errorf("named tunnel requires a config file or inline YAML")
		}
		args := []string{"tunnel", "--config", configPath, "run"}
		if name := strings.TrimSpace(s.cfg.Name); name != "" {
			args = append(args, name)
		}
		return args, nil
	case ModeQuick:
		origin := strings.TrimSpace(s.cfg.OriginURL)
		if origin == "" {
			return nil, fmt.Errorf("quick tunnel requires an origin URL")
		}
		return []string{"tunnel", "--url", origin}, nil
	default:
		return nil, fmt.Errorf("unknown tunnel mode %q", s.cfg.Mode)
	}
}

// URLFromConfig extracts the public hostname from a locally-managed tunnel config YAML, without
// starting cloudflared. Returns "" when the config declares no ingress hostname.
func URLFromConfig(yaml string) string {
	for _, rawLine := range strings.Split(yaml, "\n") {
		line := strings.TrimSpace(rawLine)
		// Ingress entries are list items: "- hostname: example.com".
		line = strings.TrimPrefix(line, "- ")
		if !strings.HasPrefix(line, "hostname:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "hostname:"))
		value = strings.Trim(value, `"'`)
		if value != "" {
			return "https://" + value
		}
	}
	return ""
}

// URLFromLogLine extracts the first https URL cloudflared prints, used for quick tunnels.
var quickURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

func URLFromLogLine(line string) string {
	return quickURLPattern.FindString(line)
}

// Start launches cloudflared and blocks until it reports readiness or the timeout elapses.
func (s *Supervisor) Start(ctx context.Context) error {
	if s.cfg.Mode == ModeOff {
		return nil
	}
	configPath, _, err := s.ResolveFiles()
	if err != nil {
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
		return err
	}
	args, err := s.buildArgs(configPath)
	if err != nil {
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
		return err
	}

	// Resolve the URL before starting when the config declares one.
	url := strings.TrimSpace(s.cfg.URL)
	if url == "" && s.cfg.Mode == ModeNamed && strings.TrimSpace(s.cfg.ConfigYAML) != "" {
		url = URLFromConfig(s.cfg.ConfigYAML)
	}
	if url == "" && s.cfg.Mode == ModeNamed && strings.TrimSpace(s.cfg.ConfigFile) != "" {
		if raw, err := os.ReadFile(s.cfg.ConfigFile); err == nil {
			url = URLFromConfig(string(raw))
		}
	}

	s.mu.Lock()
	s.url = url
	s.err = ""
	s.stopped = false
	s.mu.Unlock()

	// The tunnel outlives the request that started it. Deriving the process
	// context from the request context would make net/http cancel it the
	// moment the handler returns, killing the tunnel the operator just
	// enabled; it only survived because the supervisor relaunched it.
	// WithoutCancel keeps the request's values and drops its cancellation,
	// and the supervisor's own Stop is what ends the process.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(runCtx, s.cfg.Bin, args...)
	s.mu.Lock()
	s.cancel = cancel
	s.cmd = cmd
	s.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.pid = cmd.Process.Pid
	s.running = true
	s.start = time.Now()
	s.mu.Unlock()

	ready := make(chan struct{}, 1)
	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if s.cfg.Mode == ModeQuick {
				if found := URLFromLogLine(line); found != "" {
					s.mu.Lock()
					if s.url == "" {
						s.url = found
					}
					s.mu.Unlock()
					select {
					case ready <- struct{}{}:
					default:
					}
				}
				continue
			}
			// Named mode: readiness line + optional URL from logs.
			lower := strings.ToLower(line)
			if strings.Contains(lower, "registered tunnel connection") ||
				strings.Contains(lower, "connection registered") ||
				strings.Contains(lower, "tunnel connection") && strings.Contains(lower, "registered") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
			if url == "" {
				if found := URLFromLogLine(line); found != "" {
					s.mu.Lock()
					if s.url == "" {
						s.url = found
					}
					s.mu.Unlock()
				}
			}
		}
	}()

	select {
	case <-ready:
	case <-time.After(s.cfg.ReadinessTimeout):
		cancel()
		s.mu.Lock()
		s.err = "cloudflared did not become ready within " + s.cfg.ReadinessTimeout.String()
		s.running = false
		s.mu.Unlock()
		return fmt.Errorf("%s", s.err)
	case <-ctx.Done():
		cancel()
		s.mu.Lock()
		s.err = ctx.Err().Error()
		s.running = false
		s.mu.Unlock()
		return ctx.Err()
	}

	// Reap the process in the background. When cloudflared exits on its own
	// the tunnel is down, so the supervisor relaunches it with backoff: a
	// dropped Cloudflare connection should not need a human to click Enable.
	// A deliberate Stop sets the stopped flag, which suppresses the relaunch.
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		s.running = false
		stopped := s.stopped
		if time.Since(s.start) > stableAfter {
			// The tunnel stayed up long enough to count as healthy, so the
			// next drop starts from the first backoff step again.
			s.restarts = 0
		}
		s.mu.Unlock()
		if stopped || s.cfg.Mode == ModeOff {
			return
		}
		if err := s.Restart(context.Background()); err != nil {
			return
		}
	}()

	return nil
}

// Stop terminates the supervised cloudflared process and suppresses the
// automatic relaunch that an unexpected exit would otherwise trigger.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.stopped = true
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// Status returns a snapshot of the supervisor's state.
func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		Mode:      s.cfg.Mode,
		Running:   s.running,
		URL:       s.url,
		PID:       s.pid,
		StartedAt: s.start,
		LastError: s.err,
		Restarts:  s.restarts,
	}
}

// Restart relaunches cloudflared after an unexpected exit, with bounded backoff.
// The restart counter is reset once the tunnel has stayed up for a while, so a
// tunnel that ran for days before dropping does not start from the last
// backoff step.
func (s *Supervisor) Restart(ctx context.Context) error {
	s.mu.Lock()
	if s.restarts >= maxRestarts {
		s.err = "cloudflared restart limit reached"
		s.mu.Unlock()
		return fmt.Errorf("%s", s.err)
	}
	s.restarts++
	s.mu.Unlock()
	time.Sleep(s.backoff())
	// Stop may have landed while the backoff was running. Restarting now
	// would resurrect a tunnel the operator just disabled, so the pending
	// relaunch is dropped.
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return fmt.Errorf("cloudflared restart cancelled: supervisor stopped")
	}
	return s.Start(ctx)
}

func (s *Supervisor) backoff() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	delay := time.Second
	for i := 1; i < s.restarts && i < 5; i++ {
		delay *= 2
	}
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}
