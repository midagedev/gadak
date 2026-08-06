package server

// Delayed background-sync start for first-run onboarding: when `scry serve`
// starts without a credential, cmdServe registers a starter; the first
// successful PUT onboarding/connect/ fires it exactly once (sync.Once).

// SetSyncStarter registers a function that starts the background sync loop
// after the first credential is saved via onboarding connect. Fired at most
// once. cmdServe registers this when serve starts without a credential.
func (h *Handler) SetSyncStarter(f func()) {
	if h == nil || h.s == nil {
		return
	}
	h.s.syncStarter = f
}

// fireSyncStarterIfNeeded runs the registered starter exactly once when this
// is the first credential (hadCredential false). No-op if none is registered.
func (s *server) fireSyncStarterIfNeeded(hadCredential bool) {
	if hadCredential {
		return
	}
	f := s.syncStarter
	if f == nil {
		return
	}
	s.syncStarterOnce.Do(f)
}
