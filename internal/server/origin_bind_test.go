package server

// The passthrough target is a slot, not a one-shot latch.
//
// It became a latch in PR #69: BindOriginHandler assigned inside a
// sync.Once so a bind could not race lazy construction. Serialising the two
// was right; making the slot unreplaceable was not. The workspace mount
// binds a handler, and when its advertise fails it closes that persist
// session and the rescan loop retries with a fresh one — under the Once the
// retry's bind was a silent no-op, and the mount then ran two issuetap
// stores over one persist file. Measured on that build: two issues both
// minted id 10001, one through the pinned dead handler and one through the
// live session.
//
// A silently-ignored bind is the worst shape available here, because
// nothing fails: the request still answers 200 (issuetap's Store.Close is a
// WAL flush and the working copy stays readable), so the divergence only
// shows up in the data.

import (
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/origin"
)

func bindProbe(name string) (http.Handler, func() int) {
	reqs := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Bind-Probe", name)
		_, _ = w.Write([]byte(`{}`))
	}), func() int { return reqs }
}

func servedBy(t *testing.T, h *Handler) string {
	t.Helper()
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": basicLocalOrigin(t),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("passthrough: %d %s", rec.Code, rec.Body.String())
	}
	return rec.Header().Get("X-Bind-Probe")
}

func TestBindOriginHandlerReplacesTheTarget(t *testing.T) {
	h, _ := localOriginServer(t)
	first, firstReqs := bindProbe("first")
	second, secondReqs := bindProbe("second")

	h.BindOriginHandler(first)
	if got := servedBy(t, h); got != "first" {
		t.Fatalf("first bind not serving: %q", got)
	}
	h.BindOriginHandler(second)
	if got := servedBy(t, h); got != "second" {
		t.Fatalf("second bind ignored, still on %q — a caller that replaced a dead session is talking to the dead one", got)
	}
	if firstReqs() != 1 || secondReqs() != 1 {
		t.Fatalf("requests landed first=%d second=%d; want one each", firstReqs(), secondReqs())
	}
}

func TestBindOriginHandlerNilUnbindsBackToLazy(t *testing.T) {
	// The unbind path the mount uses when its advertise fails: drop the
	// handler wrapping the session it is about to close, so the slot goes
	// back to lazy instead of pinning a store nobody owns.
	h, _ := localOriginServer(t)
	pinned, pinnedReqs := bindProbe("pinned")
	h.BindOriginHandler(pinned)
	if got := servedBy(t, h); got != "pinned" {
		t.Fatalf("bind not serving: %q", got)
	}

	h.BindOriginHandler(nil)
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": basicLocalOrigin(t),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("after unbind the lazy path did not answer: %d %s", rec.Code, rec.Body.String())
	}
	if probe := rec.Header().Get("X-Bind-Probe"); probe != "" {
		t.Fatalf("unbind ignored, still served by %q", probe)
	}
	if pinnedReqs() != 1 {
		t.Fatalf("unbound handler took %d requests; want the one before the unbind", pinnedReqs())
	}
}

func TestBindOriginHandlerAfterLazyUseTakesEffect(t *testing.T) {
	// The mount publishes its entry before it binds, so a concurrent
	// /w/<name>/api/v1/origin/ request can construct the lazy handler
	// first. That must not lock the slot against the owner's bind.
	h, _ := localOriginServer(t)
	if got := servedBy(t, h); got != "" {
		t.Fatalf("expected the lazy handler first, got probe %q", got)
	}
	late, _ := bindProbe("late")
	h.BindOriginHandler(late)
	if got := servedBy(t, h); got != "late" {
		t.Fatalf("bind after lazy use ignored, still on %q", got)
	}
}
