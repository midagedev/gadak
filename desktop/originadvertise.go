package main

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// startStandaloneOriginListener gives a standalone desktop app the
// advertise surface `gadak serve` has (GDK-333), so a concurrent CLI
// routes writes here instead of opening a second embedded issuetap graph
// over the same persist file (GDK-340). The app itself has no TCP
// listener by design — this one is loopback-only, an OS-picked port, and
// serves exactly two paths, both forwarded to api (which carries the
// browser guard and the X-Gadak identity headers):
//
//	GET origin.ProbePath   — the port_fallback / probeMatches probe
//	origin.RESTPrefix/...  — the origin passthrough a CLI write takes
//
// Everything else is 404: this is not a UI/API server ("no forced
// server/port" invariant). Returns a cleanup that withdraws the advertise
// file and closes the listener. Connected workspaces are a no-op.
//
// Failure is fatal (GDK-343): the persist lock is taken here — before the
// advertise write — so a second owner dies at startup instead of stealing
// routing; and once this process holds the lock, a missing advertise file
// would turn every concurrent CLI write into a hard "workspace busy" error
// instead of a route, so a failed listener or advertise write aborts too.
func startStandaloneOriginListener(cfg *config.Config, api http.Handler) (func(), error) {
	nop := func() {}
	if cfg == nil || !cfg.IsStandalone() || api == nil {
		return nop, nil
	}
	dir := cfg.Directory()
	if dir == "" {
		var err error
		dir, err = config.Dir()
		if err != nil || dir == "" {
			return nop, nil
		}
	}
	// Eager persist lock: fail before advertising, not at the first write.
	if _, err := origin.StandaloneHandler(cfg); err != nil {
		return nop, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nop, fmt.Errorf("could not open origin passthrough listener: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET "+origin.ProbePath+"{$}", api)
	mux.Handle(origin.RESTPrefix+"/", api)
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("warning: origin passthrough listener: %v", err)
		}
	}()
	if err := origin.WriteAdvertise(dir, ln.Addr().String()); err != nil {
		_ = srv.Close()
		return nop, fmt.Errorf("could not advertise origin owner: %w", err)
	}
	return func() {
		_ = origin.RemoveAdvertise(dir)
		_ = srv.Close()
	}, nil
}
