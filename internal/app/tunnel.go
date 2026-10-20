package app

import (
	"context"

	"go.uber.org/fx"

	"9router/proxy/internal/tunnel/cloudflared"
)

// TunnelModule stops the supervised Cloudflare tunnel when the gateway shuts
// down.
//
// The tunnel is a child process. When the gateway exits without stopping it,
// the child is reparented to init and keeps running: the tunnel hostname
// stays routed to a port nothing is listening on, and the next start finds a
// stale connector already attached to the same named tunnel. A notebook
// runtime that is restarted regularly accumulates one of those per restart.
//
// An OnStop hook is the only place that catches every exit path, so the
// supervisor is stopped here rather than in a signal handler.
var TunnelModule = fx.Module("tunnel",
	fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				cloudflared.Shared().Stop()
				return nil
			},
		})
	}),
)
