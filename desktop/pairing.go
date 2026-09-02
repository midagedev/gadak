package main

// Settings → Devices (GDK-1047): the desktop app's half of phone pairing.
// The flow itself lives in internal/pairflow — the same code `gadak
// pairing` runs — so a mint from this tab and a mint from the terminal
// produce the identical token, offer, and _home routing-token guard. What
// this file owns is the route surface: JSON in, JSON out, and the QR as a
// PNG data URI the webview can put in an <img>.
//
// Reachability is the same story as every /desktop/* route: the mux hangs
// off the Wails asset handler, there is no TCP listener, and only the
// webview can call it. Gadak serve never mounts these.
//
// The offer is a credential. It appears in exactly one response — the
// mint's — and never in a log line, an error body, or the devices list.
// Error bodies carry stable codes, not store wording.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairflow"
	"github.com/midagedev/gadak/internal/pairing"
)

// registerPairingRoutes hangs the Devices tab's three routes on the
// desktop mux. main.go calls this once inside fallbackHandler.
func registerPairingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /desktop/pairing", handlePairingGET)
	mux.HandleFunc("POST /desktop/pairing/mint", handlePairingMint)
	mux.HandleFunc("POST /desktop/pairing/revoke", handlePairingRevoke)
}

// deviceRow is one Devices-tab row: what `gadak pairing list` prints, at
// rest. Never a token, never the full hash — the 8-char prefix is the
// floor revoke accepts.
type deviceRow struct {
	Label      string `json:"label"`
	Scope      string `json:"scope"`
	ExpiresAt  string `json:"expires_at"`
	HashPrefix string `json:"hash_prefix"`
	State      string `json:"state"`
}

// handlePairingGET answers the tab's whole world state: the device rows,
// the endpoint a mint would advertise, and whether this workspace is
// local-origin (the routing-token guard only applies to a local-origin home).
// When the workspace cannot own devices — paired away, or no credential
// yet — the answer is still 200 with empty rows and `unavailable` naming
// why, so the tab can render its disabled reason instead of guessing from
// a failed POST. advertised_endpoint empty means "no live serve found";
// the mint form treats that as "endpoint required".
func handlePairingGET(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		// No credential yet is a renderable state, not a fault: the tab
		// shows its disabled reason instead of an error banner.
		if errors.Is(err, config.ErrNotConfigured) {
			writePairingJSON(w, map[string]any{
				"devices":             []deviceRow{},
				"advertised_endpoint": "",
				"standalone":          false,
				"unavailable":         "not_configured",
			})
			return
		}
		log.Printf("pairing list: %v", err)
		writePairingErr(w, http.StatusInternalServerError, "list_failed")
		return
	}
	resp := map[string]any{
		"devices":             []deviceRow{},
		"advertised_endpoint": pairflow.AdvertisedEndpoint(cfg),
		"standalone":          cfg.HasLocalOrigin(),
	}
	dir, err := pairflow.Dir(cfg)
	if err != nil {
		if unavailable := pairingUnavailable(err); unavailable != "" {
			resp["unavailable"] = unavailable
			writePairingJSON(w, resp)
			return
		}
		log.Printf("pairing list: %v", err)
		writePairingErr(w, http.StatusInternalServerError, "list_failed")
		return
	}
	rows, err := pairflow.Rows(dir, time.Now())
	if err != nil {
		log.Printf("pairing list: %v", err)
		writePairingErr(w, http.StatusInternalServerError, "list_failed")
		return
	}
	devices := make([]deviceRow, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, deviceRow{
			Label:      row.Label,
			Scope:      row.Scope,
			ExpiresAt:  row.Expires,
			HashPrefix: row.Hash,
			State:      row.State,
		})
	}
	resp["devices"] = devices
	writePairingJSON(w, resp)
}

// handlePairingMint runs the shared device mint for the webview's form.
// The scope allowlist is origin|serve only: a terminal token opens a shell
// on this machine, and that power belongs to the person at the terminal
// who can see `gadak help pairing` — a point-and-click surface must not
// hand it out. `_home` is refused here too; rotation of the routing key
// stays a CLI verb (it has no offer to show).
func handlePairingMint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label    string `json:"label"`
		Scope    string `json:"scope"`
		TTL      string `json:"ttl"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writePairingErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	label := strings.TrimSpace(body.Label)
	if label == "" {
		writePairingErr(w, http.StatusBadRequest, "label_required")
		return
	}
	if label == pairing.HomeLabel {
		writePairingErr(w, http.StatusBadRequest, "reserved_label")
		return
	}
	scope := strings.TrimSpace(body.Scope)
	if scope == "" {
		scope = pairing.ScopeServe // the phone is the common case here
	}
	if scope != pairing.ScopeOrigin && scope != pairing.ScopeServe {
		writePairingErr(w, http.StatusBadRequest, "bad_scope")
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writePairingLoadErr(w, err)
		return
	}
	dir, err := pairflow.Dir(cfg)
	if err != nil {
		if unavailable := pairingUnavailable(err); unavailable != "" {
			writePairingErr(w, http.StatusConflict, unavailable)
			return
		}
		log.Printf("pairing mint: %v", err)
		writePairingErr(w, http.StatusInternalServerError, "mint_failed")
		return
	}
	res, err := pairflow.MintDevice(dir, cfg, label, scope, body.TTL, strings.TrimSpace(body.Endpoint), time.Now())
	if err != nil && res.Offer == "" {
		// The flow refuses before minting; classify by its own wording,
		// which never carries the credential.
		switch {
		case strings.Contains(err.Error(), "already exists"):
			writePairingErr(w, http.StatusConflict, "label_exists")
		case strings.Contains(err.Error(), "no live serve"),
			strings.Contains(err.Error(), "listens on loopback"):
			// GDK-1266: the form sent no endpoint and the live serve is
			// loopback-only — same prescription as no serve: fill it in.
			writePairingErr(w, http.StatusConflict, "no_serve")
		case strings.Contains(err.Error(), "bad endpoint"):
			writePairingErr(w, http.StatusBadRequest, "bad_endpoint")
		case strings.Contains(err.Error(), "bad ttl"):
			writePairingErr(w, http.StatusBadRequest, "bad_ttl")
		default:
			log.Printf("pairing mint: %v", err)
			writePairingErr(w, http.StatusInternalServerError, "mint_failed")
		}
		return
	}
	// The QR the phone scans: same module matrix the terminal draws,
	// rendered as a PNG data URI so the webview needs no decode step.
	png, qrErr := pairflow.QRPNG(res.Offer)
	if qrErr != nil {
		// The mint itself succeeded; the offer must still reach the user.
		log.Printf("pairing mint QR: %v", qrErr)
	}
	resp := map[string]any{
		"offer":            res.Offer,
		"label":            res.Label,
		"scope":            res.Scope,
		"endpoint":         res.Endpoint,
		"expires_at":       res.ExpiresAt,
		"hash_prefix":      res.Meta.Hash[:8],
		"loopback_warning": res.LoopbackWarning,
	}
	if qrErr == nil {
		resp["qr_png"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	// A guard failure after the mint (disk-level, rare): the credential
	// exists exactly once, in this response — withholding it would strand
	// the token, so the mint answers success with the warning attached.
	if err != nil {
		log.Printf("pairing mint (post-mint step): %v", err)
		resp["warning"] = "home_routing"
	}
	writePairingJSON(w, resp)
}

// handlePairingRevoke closes one device token by label or hash prefix —
// the same selector `gadak pairing revoke` takes. The `_home` refusal and
// the ambiguity/already-revoked wordings come from the store unchanged;
// they are classified to codes by their pinned sentences.
func handlePairingRevoke(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Selector string `json:"selector"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Selector) == "" {
		writePairingErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writePairingLoadErr(w, err)
		return
	}
	dir, err := pairflow.Dir(cfg)
	if err != nil {
		if unavailable := pairingUnavailable(err); unavailable != "" {
			writePairingErr(w, http.StatusConflict, unavailable)
			return
		}
		log.Printf("pairing revoke: %v", err)
		writePairingErr(w, http.StatusInternalServerError, "revoke_failed")
		return
	}
	meta, err := pairflow.Revoke(dir, strings.TrimSpace(body.Selector), time.Now())
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "own routing key"):
			writePairingErr(w, http.StatusBadRequest, "home_refused")
		case strings.Contains(err.Error(), "already revoked"):
			writePairingErr(w, http.StatusConflict, "already_revoked")
		case strings.Contains(err.Error(), "no token matches"):
			writePairingErr(w, http.StatusNotFound, "not_found")
		case strings.Contains(err.Error(), "matches"):
			writePairingErr(w, http.StatusBadRequest, "ambiguous")
		default:
			log.Printf("pairing revoke: %v", err)
			writePairingErr(w, http.StatusInternalServerError, "revoke_failed")
		}
		return
	}
	writePairingJSON(w, map[string]any{"revoked": meta.Label, "hash_prefix": meta.Hash[:8]})
}

// pairingUnavailable classifies the two workspace states that cannot own
// devices: paired away (its home is another machine) and no credential
// yet. Empty string means the error is something else.
func pairingUnavailable(err error) string {
	if errors.Is(err, config.ErrNotConfigured) {
		return "not_configured"
	}
	if strings.Contains(err.Error(), "run on the home machine") {
		return "paired_away"
	}
	return ""
}

// writePairingLoadErr answers a config.Load failure: unconfigured is a
// state the tab renders (409 + code), anything else is a fault (500).
func writePairingLoadErr(w http.ResponseWriter, err error) {
	if errors.Is(err, config.ErrNotConfigured) {
		writePairingErr(w, http.StatusConflict, "not_configured")
		return
	}
	log.Printf("pairing config: %v", err)
	writePairingErr(w, http.StatusInternalServerError, "load_failed")
}

func writePairingJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Mint responses carry the offer; nothing downstream may cache one.
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func writePairingErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, `{"error":`+strconv.Quote(code)+`}`, status)
}
