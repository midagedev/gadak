package origin

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/serveaddr"
)

// RESTPrefix is the serve passthrough root. A client request to
// /rest/api/3/issue is sent to <serve> + RESTPrefix + /rest/api/3/issue.
// Paired remote clients reach this machine's origin through this prefix.
const RESTPrefix = "/api/v1/origin"

// ProbePath and probeTimeout are how port_fallback and views open decide
// whether a loopback port is a gadak UI serve (X-Gadak / X-Gadak-Profile).
// They are not used to route origin writes. A leftover serve-origin.json
// from a previous version is ignored (GDK-936).
const ProbePath = "/api/v1/issues/sync/progress/"

const probeTimeout = 700 * time.Millisecond

// OwnerStatus is the doctor line for a standalone workspace. There is no
// exclusive persist owner after GDK-936 (WAL); leftover serve-origin.json
// is ignored. Empty when the workspace is not standalone.
func OwnerStatus(cfg *config.Config) string {
	if cfg == nil || !cfg.IsStandalone() {
		return ""
	}
	return "embedded (no live serve)"
}

// GadakProbe classifies a loopback GET to the progress endpoint. Exported
// so cmd/gadak's port fallback and views open use this single copy
// (GDK-423). Guards: 700ms context, no Origin header, X-Gadak required,
// profile from X-Gadak-Profile.
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

// LiveServeFor returns the first live UI serve advertising profile, found
// through the home-root run directory (serveaddr) and the identity probe —
// not leftover serve-origin.json, which GDK-936 made meaningless.
//
// This is the single owner of that discovery walk (GDK-987): until it, two
// copies of the same serveaddr.List → ProbeGadakOnPort loop lived in
// originbind.RefuseIfOpen (standalone→connected conversion refusal) and
// pairflow.AdvertisedEndpoint (the pairing endpoint default), and a guard
// added to one copy would silently miss the other. Consumers keep their own
// framing of the record: RefuseIfOpen wraps PID/Addr in WorkspaceOpenError,
// AdvertisedEndpoint upgrades Addr to a URL.
func LiveServeFor(profile string) (serveaddr.Record, bool) {
	dir, err := serveaddr.Dir()
	if err != nil {
		return serveaddr.Record{}, false
	}
	for _, rec := range serveaddr.List(dir) {
		got := ProbeGadakOnPort(rec.Port, 0)
		if !got.IsGadak {
			continue
		}
		if got.Profile != profile {
			continue
		}
		return rec, true
	}
	return serveaddr.Record{}, false
}
