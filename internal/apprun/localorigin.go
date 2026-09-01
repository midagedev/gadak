package apprun

import (
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// acquireLocalOrigin marks this process as holding an embedded origin
// session and opens it. Two processes may embed the same WAL persist
// (GDK-936); this is not an exclusive lock.
func (rt *Runtime) acquireLocalOrigin() error {
	if rt == nil || rt.Cfg == nil || !rt.Cfg.HasLocalOrigin() {
		return nil
	}
	origin.SetInProcess(rt.Cfg, true)
	if _, err := origin.LocalOriginHandler(rt.Cfg); err != nil {
		origin.SetInProcess(rt.Cfg, false)
		return err
	}
	rt.acquiredLocalOrigin = true
	return nil
}

// StartOriginPassthrough embeds the local-origin origin for this process.
// It does not open a loopback listener and does not write serve-origin.json
// (GDK-936: local writes embed the persist WAL directly; pairing remotes
// use the main serve's RESTPrefix). Connected workspaces are a no-op.
//
// The desktop caller invokes this after application.New so wails
// SingleInstance can os.Exit the second process before persist is opened
// (GDK-658).
func StartOriginPassthrough(cfg *config.Config) (func(), error) {
	if cfg == nil || !cfg.HasLocalOrigin() {
		return nopStop, nil
	}
	origin.SetInProcess(cfg, true)
	if _, err := origin.LocalOriginHandler(cfg); err != nil {
		origin.SetInProcess(cfg, false)
		return nopStop, err
	}
	note("local-origin-persist")
	return nopStop, nil
}

// StartOriginPassthrough embeds persist (if not already held). Desktop
// calls this after application.New (GDK-658).
func (rt *Runtime) StartOriginPassthrough() (func(), error) {
	if rt == nil {
		return nopStop, nil
	}
	stop, err := StartOriginPassthrough(rt.Cfg)
	if err != nil {
		return nopStop, err
	}
	if rt.Cfg != nil && rt.Cfg.HasLocalOrigin() {
		rt.acquiredLocalOrigin = true
	}
	return stop, nil
}
