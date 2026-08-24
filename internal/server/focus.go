package server

import (
	"net/http"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/uifocus"
)

// GET ui-focus/ — two signals on one cheap poll (GDK-791):
//
//   - hash: the one-shot CLI focus payload for this profile, if pending.
//     The file is consumed on read.
//   - configVersion: the disk identity of this profile's config.json. Always
//     present — this is how a `gadak config set` (or a write from another
//     tab) reaches an already-open UI with no reload: the client sees the
//     version move and refetches config.json.
//
// The response is 200 with a JSON body in both cases (204 retired: an empty
// body cannot carry the version).
func (s *server) handleUIFocus(w http.ResponseWriter, r *http.Request) {
	hash, ok, err := uifocus.TakeFor(s.profile)
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
	}
	writeJSON(w, http.StatusOK, body)
}
