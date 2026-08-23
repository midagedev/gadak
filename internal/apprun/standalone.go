package apprun

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// acquireStandalone marks this process as the persist owner and takes
// the issuetap persist lock (GDK-343). A second owner must fail here —
// before any advertise write could steal routing — not at the first write.
func (rt *Runtime) acquireStandalone() error {
	if rt == nil || rt.Cfg == nil || !rt.Cfg.IsStandalone() {
		return nil
	}
	origin.SetInProcess(true)
	if _, err := origin.StandaloneHandler(rt.Cfg); err != nil {
		origin.SetInProcess(false)
		return err
	}
	rt.acquiredStandalone = true
	return nil
}

// PublishAdvertise writes serve-origin.json for a standalone workspace and
// returns a cleanup that removes it. Connected workspaces and a missing
// profile dir are no-ops.
//
// A write failure is fatal (GDK-343 / F6): this process holds the persist
// lock, so without the advertise file every concurrent CLI write becomes a
// hard "workspace busy" error instead of routing here. Failing loud at
// startup beats a warning nobody reads.
//
// serve calls this after bind (the final address, including port fallback).
// Desktop does not: it advertises an origin-only listener via
// StartOriginPassthrough ("no forced server/port").
func PublishAdvertise(cfg *config.Config, bound string) (func(), error) {
	if cfg == nil || !cfg.IsStandalone() || bound == "" {
		return nopStop, nil
	}
	dir := profileDir(cfg)
	if dir == "" {
		return nopStop, nil
	}
	if err := origin.WriteAdvertise(dir, bound); err != nil {
		return nopStop, fmt.Errorf("could not advertise origin owner: %w", err)
	}
	note("advertise")
	log.Print("apprun: origin advertised")
	return func() { _ = origin.RemoveAdvertise(dir) }, nil
}

// StartOriginPassthrough gives a standalone desktop app the advertise
// surface `gadak serve` has (GDK-333), so a concurrent CLI routes writes
// here instead of opening a second embedded issuetap graph over the same
// persist file (GDK-340). The app itself has no TCP listener by design —
// this one is loopback-only, an OS-picked port, and serves exactly two
// paths, both forwarded to api (which carries the browser guard and the
// X-Gadak identity headers):
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
//
// The desktop caller invokes this after application.New so wails
// SingleInstance can os.Exit the second process before persist is taken
// (GDK-658). Lock-before-advertise inside this function is unchanged.
func StartOriginPassthrough(cfg *config.Config, api http.Handler) (func(), error) {
	if cfg == nil || !cfg.IsStandalone() || api == nil {
		return nopStop, nil
	}
	dir := profileDir(cfg)
	if dir == "" {
		return nopStop, nil
	}
	origin.SetInProcess(true)
	if _, err := origin.StandaloneHandler(cfg); err != nil {
		origin.SetInProcess(false)
		return nopStop, err
	}
	note("standalone-persist")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		origin.SetInProcess(false)
		return nopStop, fmt.Errorf("could not open origin passthrough listener: %w", err)
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
		origin.SetInProcess(false)
		return nopStop, fmt.Errorf("could not advertise origin owner: %w", err)
	}
	note("advertise")
	log.Print("apprun: origin advertised")
	return func() {
		_ = origin.RemoveAdvertise(dir)
		_ = srv.Close()
	}, nil
}

// StartOriginPassthrough takes persist (if not already held) and advertises
// an origin-only listener. Desktop calls this after application.New (GDK-658).
func (rt *Runtime) StartOriginPassthrough() (func(), error) {
	if rt == nil {
		return nopStop, nil
	}
	stop, err := StartOriginPassthrough(rt.Cfg, rt.API)
	if err != nil {
		return nopStop, err
	}
	if rt.Cfg != nil && rt.Cfg.IsStandalone() {
		rt.acquiredStandalone = true
		rt.stopOrigin = stop
	}
	return stop, nil
}
