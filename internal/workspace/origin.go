package workspace

import (
	"errors"

	"github.com/midagedev/gadak/internal/origin"
)

// testBeforeOriginAcquire holds an ownership attempt after it becomes visible
// to Registry.Close. Tests use it to prove Close waits for the attempt.
var testBeforeOriginAcquire func(name string)

// ensureOrigin makes this process the advertised persist owner of a
// standalone mount (STD-3): a process that acquires the persist lock must
// advertise a routable origin or every CLI write to the mount hard-fails
// ErrWorkspaceBusy. Connected mounts and persists already owned in-process
// (the primary, door A) are no-ops. Busy means another process owns — also a
// no-op: routing to its advertise is the working state. Idempotent; safe from
// the rescan loop.
func (r *Registry) ensureOrigin(name string, e *Entry) {
	if r == nil || e == nil || e.Cfg == nil || !e.Cfg.IsStandalone() {
		return
	}
	if origin.IsInProcess(e.Cfg) {
		return
	}

	r.mu.Lock()
	if r.closed || e.stopOrigin != nil {
		r.mu.Unlock()
		return
	}
	if f := r.owningOrigin[name]; f != nil {
		r.mu.Unlock()
		<-f.done
		return
	}
	f := &originFlight{done: make(chan struct{})}
	r.owningOrigin[name] = f
	logf := r.watchLogf
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.owningOrigin, name)
		close(f.done)
		r.mu.Unlock()
	}()

	if testBeforeOriginAcquire != nil {
		testBeforeOriginAcquire(name)
	}

	cfg := e.Cfg
	h, err := origin.StandaloneHandler(cfg)
	if err != nil {
		if !errors.Is(err, origin.ErrWorkspaceBusy) && logf != nil {
			logf("workspace " + name + ": origin owner: " + err.Error())
		}
		return
	}
	// Only a successful embedded session suppresses advertise routing. A
	// peer's lock must leave callers free to route through that peer.
	origin.SetInProcess(cfg, true)
	// Bind before the entry advertises ownership. Requests can read a published
	// entry concurrently; server.Handler serializes this binding with lazy use.
	e.Handler.BindOriginHandler(h)

	stop, err := origin.ServeOriginPassthrough(cfg.Directory(), e.Handler)

	if err != nil {
		// A held lock without an advertise is exactly the defect (STD-3):
		// release the lock so a CLI can embed; the rescan loop retries.
		// Unbind first: the handler we just pinned wraps the session about
		// to be closed, and leaving it pinned means the retry's fresh
		// session and this dead one are two stores over one persist file.
		e.Handler.BindOriginHandler(nil)
		_ = origin.CloseStandalone(cfg)
		origin.SetInProcess(cfg, false)
		if logf != nil {
			logf("workspace " + name + ": origin advertise: " + err.Error())
		}
		return
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		stop()
		e.Handler.BindOriginHandler(nil)
		_ = origin.CloseStandalone(cfg)
		origin.SetInProcess(cfg, false)
		return
	}
	e.stopOrigin = stop
	e.ownsOrigin = true
	r.mu.Unlock()
	if logf != nil {
		logf("workspace " + name + ": origin advertised")
	}
}

// EnsureOrigins retries ownership for every opened entry. The rescan loop
// calls this so a failed advertise (or a lazily re-taken lock after a
// rollback) heals within watchRescanInterval.
func (r *Registry) EnsureOrigins() {
	if r == nil {
		return
	}
	r.mu.Lock()
	snap := make(map[string]*Entry, len(r.entries))
	for name, entry := range r.entries {
		snap[name] = entry
	}
	r.mu.Unlock()
	for name, entry := range snap {
		r.ensureOrigin(name, entry)
	}
}
