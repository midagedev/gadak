package workspace

// POST /api/v1/workspaces and DELETE /api/v1/workspaces/{name} — the serve
// side of workspace management (GDK-1096). The removal contract is
// remove.go's Remove (shared with the CLI verb); creation reuses
// originbind.SeedStandalone, the same core `gadak init --standalone` and
// POST onboarding/standalone seed through, so the web cannot mint a
// workspace shape the CLI paths would treat differently.
//
// No Host/Origin/token checks live here: these handlers sit on the outer
// serve mux, where server.GuardBrowser wraps the whole tree, and neither
// Host exemption (PairedOriginHostExempt, PairedMirrorHostExempt) admits
// these paths — a DNS-named Host answers 403 before the handler runs
// (proven by the boundary tests next to buildServeMux).

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
)

// createWorkspaceDoc is the POST /api/v1/workspaces body. kind must be
// spelled by the caller, not defaulted: only standalone is creatable over
// HTTP — connected needs a credential flow and paired needs the pairing
// flow, and both stay off this API rather than half-existing here.
type createWorkspaceDoc struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Projects string `json:"projects"`
}

// CreateHandler answers POST /api/v1/workspaces by seeding a standalone
// workspace profile. The seed is the shared one: config write, default issue
// type, mirror fill. A fill that fails is logged, not failed (SeedStandalone
// contract — the workspace exists and writes work; the next sync fills).
func CreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in createWorkspaceDoc
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			manageFail(w, http.StatusBadRequest, "invalid_body")
			return
		}
		if in.Kind != config.KindStandalone {
			manageFail(w, http.StatusBadRequest, "unsupported_kind")
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		// "" and "default" name the root workspace, and the root is the
		// onboarding flow's to fill — not this API's to reseed.
		if in.Name == "" || in.Name == "default" {
			manageFail(w, http.StatusBadRequest, "invalid_name")
			return
		}
		// DirFor is the single owner of name validation (same owner Remove
		// uses): separators, "..", and other escapes die here.
		dir, err := config.DirFor(in.Name)
		if err != nil {
			manageFail(w, http.StatusBadRequest, "invalid_name")
			return
		}
		if _, err := os.Stat(dir); err == nil {
			manageFail(w, http.StatusConflict, "exists")
			return
		} else if !os.IsNotExist(err) {
			log.Printf("workspaces: create %s: stat: %v", in.Name, err)
			manageFail(w, http.StatusInternalServerError, "create_failed")
			return
		}

		// LoadFor on a missing profile returns a dir-bound empty Config
		// (not an error); Save inside the seed writes it into the new
		// profile directory.
		cfg, err := config.LoadFor(in.Name)
		if err != nil {
			log.Printf("workspaces: create %s: load: %v", in.Name, err)
			manageFail(w, http.StatusInternalServerError, "create_failed")
			return
		}
		// The opener holds this new profile's mirror for the fill only —
		// the registry's entries (the serving workspaces) are not touched,
		// and the handle is released as soon as the fill finishes (same
		// coordinates Registry.construct opens with; the CLI's
		// initStandalone does the same open/close).
		openMirror := func() (*store.DB, func() error, error) {
			dbPath, err := config.DBPathFor(in.Name)
			if err != nil {
				return nil, nil, err
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return nil, nil, err
			}
			return db, db.Close, nil
		}
		fillErr, err := originbind.SeedStandalone(cfg, in.Projects, openMirror)
		if err != nil {
			log.Printf("workspaces: create %s: seed: %v", in.Name, err)
			manageFail(w, http.StatusInternalServerError, "create_failed")
			return
		}
		if fillErr != nil {
			log.Printf("workspaces: create %s: could not fill the mirror yet: %v", in.Name, fillErr)
		}
		// Flush only this profile's origin session. origin.Close() is the
		// process-exit verb: it would checkpoint every live session,
		// including the ones the running serve is writing through.
		if err := origin.CloseStandalone(cfg); err != nil {
			log.Printf("workspaces: create %s: flush origin persist: %v", in.Name, err)
			manageFail(w, http.StatusInternalServerError, "origin_flush_failed")
			return
		}

		_, persist := origin.Describe(cfg) // same owner the rm success line uses
		writeManageJSON(w, http.StatusCreated, map[string]string{
			"name":    in.Name,
			"kind":    config.KindStandalone,
			"persist": persist,
		})
	}
}

// RemoveHandler answers DELETE /api/v1/workspaces/{name}?yes=1&destroy_origin=1.
// Refusals map from Remove's typed errors; detail reuses the CLI refusal
// wording so both surfaces teach the same next move.
func RemoveHandler(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		yes := manageQueryBool(r, "yes")
		destroyOrigin := manageQueryBool(r, "destroy_origin")

		// HTTP-only refusal, deliberately asymmetric with the CLI: the CLI
		// lets you remove the workspace you are in (advisories explain the
		// retarget), but a serve deleting the profile it is serving would
		// pull its own mirror and origin session out from under the
		// running mounts. That removal belongs to a terminal.
		if sameProfile(config.Profile(), name) {
			manageFailDetail(w, http.StatusBadRequest, "self_delete",
				fmt.Sprintf("%q is the workspace this serve is running on — stop serve first, then remove it from a terminal: gadak workspaces rm %s", name, name))
			return
		}

		// Close the registry's handle (when the mount was ever opened)
		// before the directory goes: an open SQLite handle over a deleted
		// directory leaves /w/<name>/ serving a zombie. Nil registry (CLI
		// tests, desktop without one) skips this.
		reg.Evict(name)

		res, err := Remove(name, yes, destroyOrigin, "workspaces")
		if err != nil {
			manageRemoveError(w, name, err)
			return
		}
		writeManageJSON(w, http.StatusOK, map[string]any{
			"removed":          res.Removed,
			"kind":             res.Kind,
			"origin_destroyed": res.OriginDestroyed,
			"advisories":       res.Advisories(),
		})
	}
}

// manageRemoveError maps Remove's typed refusals to the HTTP contract:
// root → root_workspace, missing → not_found (404), unreadable kind →
// kind_unreadable, persist protection → needs_destroy_origin (detail
// carries the persist path, which the CLI line already embeds), missing
// confirmation → needs_yes. Unknown failures are 500s, except name
// validation which is the caller's 400.
func manageRemoveError(w http.ResponseWriter, name string, err error) {
	var root *RootRefusalError
	if errors.As(err, &root) {
		manageFailDetail(w, http.StatusBadRequest, "root_workspace", root.Error())
		return
	}
	var missing *NotFoundError
	if errors.As(err, &missing) {
		manageFail(w, http.StatusNotFound, "not_found")
		return
	}
	var unreadable *KindUnreadableError
	if errors.As(err, &unreadable) {
		manageFailDetail(w, http.StatusBadRequest, "kind_unreadable", unreadable.Error())
		return
	}
	var protect *NeedsDestroyOriginError
	if errors.As(err, &protect) {
		manageFailDetail(w, http.StatusBadRequest, "needs_destroy_origin", protect.Error())
		return
	}
	var confirm *NeedsYesError
	if errors.As(err, &confirm) {
		manageFailDetail(w, http.StatusBadRequest, "needs_yes", confirm.Error())
		return
	}
	var invalid *InvalidNameError
	if errors.As(err, &invalid) {
		manageFail(w, http.StatusBadRequest, "invalid_name")
		return
	}
	log.Printf("workspaces: remove %s: %v", name, err)
	manageFail(w, http.StatusInternalServerError, "remove_failed")
}

// manageQueryBool reads a DELETE query flag: absent or unrecognized is
// false; "1" and "true" (case-insensitive) commit.
func manageQueryBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key))) {
	case "1", "true":
		return true
	}
	return false
}

// writeManageJSON / manageFail mirror the server package's writeJSON/fail
// shape (those are unexported there; ListHandler already carries this
// package's own copy of the same three header lines).
func writeManageJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("workspaces: write response: %v", err)
	}
}

func manageFail(w http.ResponseWriter, status int, code string) {
	writeManageJSON(w, status, map[string]string{"error": code})
}

func manageFailDetail(w http.ResponseWriter, status int, code, detail string) {
	writeManageJSON(w, status, map[string]string{"error": code, "detail": detail})
}
