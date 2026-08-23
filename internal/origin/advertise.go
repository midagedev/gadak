package origin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
)

// AdvertiseRel is the profile-relative runtime file a standalone serve
// writes so other processes can find the persist owner. Connected
// workspaces never write it.
const AdvertiseRel = "serve-origin.json"

// RESTPrefix is the serve passthrough root. A client request to
// /rest/api/3/issue is sent to <serve> + RESTPrefix + /rest/api/3/issue.
const RESTPrefix = "/api/v1/origin"

// ProbePath and probeTimeout match cmd/gadak/port_fallback.go. Origin
// cannot import cmd, so the values and the header contract (X-Gadak,
// X-Gadak-Profile) are copied here including the 700ms bound — a
// longer timeout would stall every CLI write when no serve is up.
// Exported so the desktop app's origin-only listener (GDK-340) can serve
// exactly the path the probe hits.
const ProbePath = "/api/v1/issues/sync/progress/"

const probeTimeout = 700 * time.Millisecond

// Advertise is the JSON document in AdvertiseRel.
type Advertise struct {
	Addr      string `json:"addr"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

// AdvertisePath is the absolute runtime-file path inside a profile directory.
func AdvertisePath(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, AdvertiseRel)
}

// WriteAdvertise publishes the final listen address of a standalone serve.
// Atomic write (temp + rename) at 0600, same as config.Save.
func WriteAdvertise(dir, addr string) error {
	if dir == "" || addr == "" {
		return fmt.Errorf("origin: advertise: dir and addr are required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("origin: advertise dir: %w", err)
	}
	doc := Advertise{
		Addr:      addr,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	p := AdvertisePath(dir)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// RemoveAdvertise deletes the runtime file. Missing is not an error —
// crash leftover is the other path, and the next probe treats a dead
// file as "no live serve".
func RemoveAdvertise(dir string) error {
	p := AdvertisePath(dir)
	if p == "" {
		return nil
	}
	err := os.Remove(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// OwnerStatus is the doctor line: "serve pid=… addr=…" or
// "embedded (no live serve)". Empty when the workspace is not standalone.
func OwnerStatus(cfg *config.Config) string {
	if cfg == nil || !cfg.IsStandalone() {
		return ""
	}
	adv, ok := readAdvertise(cfg)
	if !ok {
		return "embedded (no live serve)"
	}
	if !probeMatches(adv, profileNameOf(cfg)) {
		return "embedded (no live serve)"
	}
	return fmt.Sprintf("serve pid=%d addr=%s", adv.PID, adv.Addr)
}

func readAdvertise(cfg *config.Config) (Advertise, bool) {
	dir, err := profileDir(cfg)
	if err != nil || dir == "" {
		return Advertise{}, false
	}
	data, err := os.ReadFile(AdvertisePath(dir))
	if err != nil {
		return Advertise{}, false
	}
	var adv Advertise
	if err := json.Unmarshal(data, &adv); err != nil {
		return Advertise{}, false
	}
	if adv.Addr == "" {
		return Advertise{}, false
	}
	return adv, true
}

func routedJira(cfg *config.Config) (*jira.Client, bool) {
	tr, ok := routedTransport(cfg)
	if !ok {
		return nil, false
	}
	c := Connected("", inProcessUser, inProcessSecret)
	if c.HTTP == nil {
		c.HTTP = &http.Client{}
	}
	c.HTTP.Transport = tr
	return c, true
}

func routedWiki(cfg *config.Config) (*confluence.Client, bool) {
	tr, ok := routedTransport(cfg)
	if !ok {
		return nil, false
	}
	w := confluence.New("", inProcessUser, inProcessSecret)
	if w.HTTP == nil {
		w.HTTP = &http.Client{}
	}
	w.HTTP.Transport = tr
	return w, true
}

func routedTransport(cfg *config.Config) (*serveOriginTransport, bool) {
	if inProcess.Load() {
		return nil, false
	}
	adv, ok := readAdvertise(cfg)
	if !ok {
		return nil, false
	}
	if !probeMatches(adv, profileNameOf(cfg)) {
		return nil, false
	}
	host, ok := loopbackHost(adv.Addr)
	if !ok {
		return nil, false
	}
	tr := newServeOriginTransport(host)
	// Once this profile's serve has a pairing token minted, its own routed
	// writes need one too — the gate has no loopback bypass (GDK-433). The
	// token comes from the same stored pairing credential the remote side
	// uses; no file means no gate (or none minted yet) and routing stays
	// byte-identical to before.
	if dir, err := profileDir(cfg); err == nil {
		tr.bearer = localRoutingToken(dir)
	}
	// The CLI's own actor rides the passthrough (GDK-586): the serve
	// forwards the header, so a write routed to a live serve attributes to
	// the writing process's agent, not the serve's identity.
	if a, ok := config.ResolveActor(cfg); ok {
		tr.actor, tr.actorName = a.Slug, a.Name
	}
	return tr, true
}

// AdvertisedAddr reports the address a live serve for this profile
// advertised, or "" when no live serve owns the workspace. Exported for
// `gadak pairing mint`, whose default --endpoint is that address: the
// thing a same-machine serve is actually listening on, verified by the
// same probe the router trusts.
func AdvertisedAddr(cfg *config.Config) string {
	adv, ok := readAdvertise(cfg)
	if !ok {
		return ""
	}
	if !probeMatches(adv, profileNameOf(cfg)) {
		return ""
	}
	return adv.Addr
}

func probeMatches(adv Advertise, profile string) bool {
	host, ok := loopbackHost(adv.Addr)
	if !ok {
		return false
	}
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	p := ProbeGadakOnPort(port, probeTimeout)
	return p.IsGadak && p.Profile == profile
}

// loopbackHost turns a listen address into host:port for a loopback dial.
// Empty / wildcard hosts become 127.0.0.1 so a bind of :7998 is still
// reachable the way port_fallback probes 127.0.0.1.
func loopbackHost(addr string) (string, bool) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", false
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), true
}

// GadakProbe classifies a loopback GET to the progress endpoint. Exported
// so cmd/gadak's port fallback uses this single copy (GDK-423). Guards:
// 700ms context, no Origin header, X-Gadak required, profile from
// X-Gadak-Profile.
type GadakProbe struct {
	IsGadak bool
	Profile string
}

func ProbeGadakOnPort(port string, timeout time.Duration) GadakProbe {
	if timeout <= 0 {
		timeout = probeTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := "http://" + net.JoinHostPort("127.0.0.1", port) + ProbePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GadakProbe{}
	}
	// Deliberately no Origin (and no custom User-Agent). Bound the client
	// as well as the request context so a hang that ignores ctx cannot
	// outlive timeout (http.DefaultClient has none).
	client := &http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return GadakProbe{}
	}
	defer res.Body.Close()
	if res.Header.Get("X-Gadak") == "" {
		return GadakProbe{}
	}
	return GadakProbe{
		IsGadak: true,
		Profile: res.Header.Get("X-Gadak-Profile"),
	}
}
