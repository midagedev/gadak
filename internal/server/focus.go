package server

import (
	"log"
	"net/http"
	"sync"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/uifocus"
)

// lastFocusLog maps a profile to the `at` most recently logged for it.
// Clients poll this handler every 500ms; a given payload is logged once per
// process. It is a map and not a single slot because two workspace mounts
// each holding a fresh payload would otherwise take turns evicting each
// other and log on every poll — the exact spam this exists to prevent. It is
// bounded by the number of mounted profiles.
var (
	focusLogMu   sync.Mutex
	lastFocusLog = map[string]string{}
)

// GET ui-focus/ — two signals on one cheap poll (GDK-791 / GDK-960):
//
//   - hash, at: the CLI focus payload for this profile, if still fresh
//     (MaxAge). The file is not consumed on read; every polling UI sees
//     the same payload. Each client applies a given at once.
//   - configVersion: the disk identity of this profile's config.json. Always
//     present — this is how a `gadak config set` (or a write from another
//     tab) reaches an already-open UI with no reload: the client sees the
//     version move and refetches config.json.
//
// The response is 200 with a JSON body in both cases (204 retired: an empty
// body cannot carry the version).
func (s *server) handleUIFocus(w http.ResponseWriter, r *http.Request) {
	hash, at, ok, err := uifocus.PeekFor(s.profile)
	if err != nil {
		serverError(w, r, err)
		return
	}
	version := ""
	if d, err := config.DirFor(s.profile); err == nil {
		version = config.ConfigVersionOfDir(d)
	}
	body := map[string]string{"configVersion": version}
	if ok {
		body["hash"] = hash
		body["at"] = at
		logUIFocusOnce(s.profile, hash, at)
	}
	writeJSON(w, http.StatusOK, body)
}

func logUIFocusOnce(profile, hash, at string) {
	focusLogMu.Lock()
	defer focusLogMu.Unlock()
	if lastFocusLog[profile] == at {
		return
	}
	lastFocusLog[profile] = at
	log.Printf("ui-focus: profile=%q hash=%q at=%s", profile, hash, at)
}
