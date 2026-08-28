package origin

import (
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/midagedev/gadak/internal/serveaddr"
)

// serveLive records a UI-serve in serveaddr and serves the identity probe
// for profile — or, with gadak false, a plain 200 without the X-Gadak
// headers, the shape a non-gadak process squatting a recorded port answers
// with. Same shape as cmd/gadak's occupyLiveServe; that helper lives in
// another package's test binary, so this is a copy, not a reuse.
func serveLive(t *testing.T, profile string, gadak bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	mux := http.NewServeMux()
	mux.HandleFunc(ProbePath, func(w http.ResponseWriter, r *http.Request) {
		if gadak {
			w.Header().Set("X-Gadak", "1")
			w.Header().Set("X-Gadak-Profile", profile)
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, ln.Addr().String(), profile); err != nil {
		t.Fatal(err)
	}
	return ln.Addr().String()
}

// TestLiveServeFor is GDK-987's recurrence gate: the single discovery owner
// returns the matching record and walks past both a port that is not a
// gadak serve and a live serve advertising another profile. Before the
// extraction these two skips lived as two copies (originbind, pairflow).
func TestLiveServeFor(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	serveLive(t, "not-gadak", false)
	serveLive(t, "other", true)
	addr := serveLive(t, "want", true)

	rec, ok := LiveServeFor("want")
	if !ok {
		t.Fatal("LiveServeFor(want) missed a live serve advertising that profile")
	}
	if rec.Addr != addr {
		t.Fatalf("LiveServeFor(want).Addr = %q, want %q", rec.Addr, addr)
	}
	if rec.PID != os.Getpid() {
		t.Fatalf("LiveServeFor(want).PID = %d, want %d (RefuseIfOpen reports it)", rec.PID, os.Getpid())
	}
	if _, ok := LiveServeFor("absent"); ok {
		t.Fatal("LiveServeFor(absent) found a serve no record advertises")
	}
}
