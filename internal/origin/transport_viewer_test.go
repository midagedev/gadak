package origin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The viewer-actor override (GDK-1966): a request that arrived through a
// loopback proxy attesting a person's identity carries that person as the
// per-request actor, and it wins over the session actor the transport was
// built with. Only the embedded (in-process) transport honors it — a paired
// serve forwards nothing on its own.

func TestHandlerTransportViewerActorOverridesSession(t *testing.T) {
	var gotActor, gotName string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = r.Header.Get("X-Issuetap-Actor")
		gotName = r.Header.Get("X-Issuetap-Actor-Name")
		w.WriteHeader(http.StatusOK)
	})
	tr := &handlerTransport{h: h, actor: "claude:354bff2b", actorName: "Claude Code"}

	roundTrip := func(ctx context.Context) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil)
		if ctx != nil {
			req = req.WithContext(ctx)
		}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	// Session actor alone: stamped, as before this override existed.
	roundTrip(nil)
	if gotActor != "claude:354bff2b" || gotName != "Claude Code" {
		t.Fatalf("session actor = %q/%q, want the transport's own", gotActor, gotName)
	}

	// The viewer override wins.
	roundTrip(WithViewerActor(context.Background(), "kim", "Kim", ""))
	if gotActor != "kim" || gotName != "Kim" {
		t.Fatalf("viewer override = %q/%q, want kim/Kim", gotActor, gotName)
	}

	// An empty-slug override is no override at all.
	roundTrip(WithViewerActor(context.Background(), "", "", ""))
	if gotActor != "claude:354bff2b" {
		t.Fatalf("empty override = %q, want the session actor back", gotActor)
	}
}

// A transport with no session actor still stamps a viewer override — the
// loopback serve's own common case (no GADAK_ACTOR, no config actor).
func TestHandlerTransportViewerActorWithoutSession(t *testing.T) {
	var gotActor string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = r.Header.Get("X-Issuetap-Actor")
		w.WriteHeader(http.StatusOK)
	})
	tr := &handlerTransport{h: h}
	req := httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil)
	req = req.WithContext(WithViewerActor(req.Context(), "kim", "", ""))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotActor != "kim" {
		t.Fatalf("actor = %q, want kim from the override alone", gotActor)
	}
}
