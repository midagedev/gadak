package main

import (
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
func startStandaloneOriginListener(cfg *config.Config, api http.Handler) func() {
	nop := func() {}
	if cfg == nil || !cfg.IsStandalone() || api == nil {
		return nop
	}
	dir := cfg.Directory()
	if dir == "" {
		var err error
		dir, err = config.Dir()
		if err != nil || dir == "" {
			return nop
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("warning: could not open origin passthrough listener: %v", err)
		return nop
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
		log.Printf("warning: could not advertise origin owner: %v", err)
		_ = srv.Close()
		return nop
	}
	return func() {
		_ = origin.RemoveAdvertise(dir)
		_ = srv.Close()
	}
}
