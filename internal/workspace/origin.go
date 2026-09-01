package workspace

import (
	"github.com/midagedev/gadak/internal/origin"
)

// testBeforeOriginAcquire holds an ownership attempt after it becomes visible
// to Registry.Close. Tests use it to prove Close waits for the attempt.
var testBeforeOriginAcquire func(name string)

// ensureOrigin binds this process's embedded local-origin origin onto a
// mounted workspace so pairing remotes hitting /w/<name>/api/v1/origin/
// land on the persist. Connected mounts and persists already owned
// in-process (the primary) are no-ops. Idempotent; safe from the rescan
// loop. Local CLI writes embed the same WAL file directly (GDK-936) —
// this no longer advertises a loopback listener.
func (r *Registry) ensureOrigin(name string, e *Entry) {
	if r == nil || e == nil || e.Cfg == nil || !e.Cfg.HasLocalOrigin() {
		return
	}
	if origin.IsInProcess(e.Cfg) {
		return
	}

	r.mu.Lock()
	if r.closed || e.ownsOrigin {
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
	h, err := origin.LocalOriginHandler(cfg)
	if err != nil {
		if logf != nil {
			logf("workspace " + name + ": origin: " + err.Error())
		}
		return
	}
	origin.SetInProcess(cfg, true)
	e.Handler.BindOriginHandler(h)

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		e.Handler.BindOriginHandler(nil)
		_ = origin.CloseLocalOrigin(cfg)
		origin.SetInProcess(cfg, false)
		return
	}
	e.ownsOrigin = true
	r.mu.Unlock()
}

// EnsureOrigins retries ownership for every opened entry. The rescan loop
// calls this so a failed bind heals within watchRescanInterval.
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
