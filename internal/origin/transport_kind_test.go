package origin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// GDK-1973: the origin learns the kind. A person's requests carry
// X-Issuetap-Actor-Type: person (issuetap a73438ee provisions such an
// account as a human); every other shape sends nothing — the header's
// absence is the agent default, so the pre-person paths stay byte-identical.

func TestHandlerTransportStampsActorType(t *testing.T) {
	var gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("X-Issuetap-Actor-Type")
		w.WriteHeader(http.StatusOK)
	})

	tr := &handlerTransport{h: h, actor: "person:kim", actorName: "Kim", actorKind: config.ActorKindPerson}
	resp, err := tr.RoundTrip(httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotType != "person" {
		t.Fatalf("person session Actor-Type = %q, want person", gotType)
	}

	// An agent session (and the empty default) sends no header at all.
	for _, tc := range []struct{ name, kind string }{
		{"agent", config.ActorKindAgent},
		{"unset", ""},
	} {
		tr := &handlerTransport{h: h, actor: "claude:354bff2b", actorKind: tc.kind}
		resp, err := tr.RoundTrip(httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if gotType != "" {
			t.Fatalf("%s session Actor-Type = %q, want no header", tc.name, gotType)
		}
	}
}

// The viewer override carries its kind with it, so a declared or attested
// person riding one request is typed as a person even on an agent session —
// and an agent override on a person session types as an agent.
func TestHandlerTransportViewerOverrideCarriesKind(t *testing.T) {
	var gotActor, gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = r.Header.Get("X-Issuetap-Actor")
		gotType = r.Header.Get("X-Issuetap-Actor-Type")
		w.WriteHeader(http.StatusOK)
	})
	tr := &handlerTransport{h: h, actor: "claude:354bff2b", actorKind: config.ActorKindAgent}

	req := httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil)
	req = req.WithContext(WithViewerActor(req.Context(), "person:kim", "Kim", config.ActorKindPerson))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotActor != "person:kim" || gotType != "person" {
		t.Fatalf("viewer override = %q type %q, want person:kim / person", gotActor, gotType)
	}
}

// The paired serve transport stamps the same header on the rewritten
// request: the home serve's passthrough forwards it, so a person on a
// paired workspace is provisioned as a person on the home origin too.
func TestServeTransportStampsActorType(t *testing.T) {
	var gotType string
	rt := headerProbe{fn: func(r *http.Request) {
		gotType = r.Header.Get("X-Issuetap-Actor-Type")
	}}
	tr := &serveOriginTransport{host: "127.0.0.1:1", bearer: "t", actor: "person:kim", actorName: "Kim", actorKind: config.ActorKindPerson, rt: rt}
	resp, err := tr.RoundTrip(httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotType != "person" {
		t.Fatalf("paired person Actor-Type = %q, want person", gotType)
	}

	tr = &serveOriginTransport{host: "127.0.0.1:1", bearer: "t", actor: "claude:354bff2b", rt: rt}
	resp, err = tr.RoundTrip(httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotType != "" {
		t.Fatalf("paired agent Actor-Type = %q, want no header", gotType)
	}
}

// headerProbe is a RoundTripper that records request headers and answers
// an empty 200 — enough for the stamping assertions above.
type headerProbe struct {
	fn func(*http.Request)
}

func (p headerProbe) RoundTrip(r *http.Request) (*http.Response, error) {
	p.fn(r)
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    r,
	}, nil
}
