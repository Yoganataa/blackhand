package cloudflared

import (
	"os"
	"sync"
)

// The tunnel supervisor is a process-level resource: exactly one cloudflared
// process belongs to one gateway, no matter how many handlers or route tables
// exist. A per-handler supervisor would let two handlers disagree about
// whether the tunnel is up, and would spawn a second cloudflared when both
// were asked to enable it.
//
// A package-level singleton is therefore the correct model, not a shortcut.
// SetShared exists so a test can install a supervisor built from a fake
// cloudflared, and restore the previous one afterwards.
var (
	sharedMu sync.Mutex
	shared   *Supervisor
)

// Shared returns the process-wide supervisor, building it from the environment
// on first use. The mode comes from CF_TUNNEL_MODE and defaults to "off", so
// a gateway with no tunnel configured gets a supervisor that does nothing.
func Shared() *Supervisor {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared == nil {
		shared = New(ConfigFromEnv(os.Getenv("DATA_DIR")))
	}
	return shared
}

// SetShared installs s as the process-wide supervisor and returns a function
// that restores the previous one. It is meant for tests; production code
// reads the supervisor through Shared.
func SetShared(s *Supervisor) (restore func()) {
	sharedMu.Lock()
	previous := shared
	shared = s
	sharedMu.Unlock()
	return func() {
		sharedMu.Lock()
		shared = previous
		sharedMu.Unlock()
	}
}
