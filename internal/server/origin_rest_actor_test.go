package server

// The serve passthrough must keep X-Issuetap-Actor/-Name (GDK-586) on its
// way to the embedded issuetap handler — with and without the pairing gate.
// The gate rewrites Authorization (that is its job); the actor header is
// the caller's identity channel and is not the gate's to touch.

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

type actorCapture struct {
	mu        sync.Mutex
	actor     string
	actorName string
	auth      string
	reqs      int
}

func (c *actorCapture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.reqs++
		c.actor = r.Header.Get("X-Issuetap-Actor")
		c.actorName = r.Header.Get("X-Issuetap-Actor-Name")
		c.auth = r.Header.Get("Authorization")
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
}

func TestOriginRESTPreservesActorHeaderWithoutGate(t *testing.T) {
	h, _ := localOriginServer(t)
	cap := &actorCapture{}
	h.BindOriginHandler(cap.handler())
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization":         basicLocalOrigin(t),
		"X-Issuetap-Actor":      "claude:354bff2b",
		"X-Issuetap-Actor-Name": "Claude (build 1)",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("passthrough: %d %s", rec.Code, rec.Body.String())
	}
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if cap.actor != "claude:354bff2b" || cap.actorName != "Claude (build 1)" {
		t.Fatalf("origin saw actor=%q name=%q; the passthrough dropped the actor headers", cap.actor, cap.actorName)
	}
}

func TestPairingGateKeepsActorHeaderWhileRewritingAuth(t *testing.T) {
	h, cfg := localOriginServer(t)
	cap := &actorCapture{}
	h.BindOriginHandler(cap.handler())

	dir := cfg.Directory()
	token, _, err := pairing.Mint(dir, "laptop", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization":         "Bearer " + token,
		"X-Issuetap-Actor":      "claude:354bff2b",
		"X-Issuetap-Actor-Name": "Claude (build 1)",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("gated passthrough: %d %s", rec.Code, rec.Body.String())
	}
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if cap.actor != "claude:354bff2b" || cap.actorName != "Claude (build 1)" {
		t.Fatalf("origin saw actor=%q name=%q; the gate must forward the actor headers untouched", cap.actor, cap.actorName)
	}
	if want := "Basic " + origin.InProcessAuthB64(); cap.auth != want {
		t.Fatalf("origin Authorization=%q, want the rewritten in-process Basic (the token never reaches the origin)", cap.auth)
	}
}
