package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// TestPairedWikiPagesLandInMirror is the GDK-1276 evidence for the transport
// half of the claim: a page created on the home machine's built-in tracker
// reaches a paired workspace's mirror through the serve passthrough, once the
// paired config carries the block pairing now writes. Two profiles under one
// GADAK_HOME — "home" owns the persist, "laptop" is paired to it — and the
// serve is the passthrough alone: no pairing tokens, so the gate is off and,
// like pairingGate on accept, the fixture presents the in-process Basic to
// the embedded handler.
func TestPairedWikiPagesLandInMirror(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GADAK_HOME", root)
	t.Setenv("HOME", root)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	home, err := config.LoadFor("home")
	if err != nil {
		t.Fatal(err)
	}
	home.Kind = config.KindLocalOrigin
	home.Confluence = origin.DefaultConfluenceConfig()
	if err := home.Save(); err != nil {
		t.Fatal(err)
	}
	hw, err := origin.Wiki(home)
	if err != nil {
		t.Fatal(err)
	}
	created, err := hw.CreatePage(context.Background(), origin.DefaultSpaceKey, "Home wiki note", wikiADF("written on the home machine"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	h, err := origin.LocalOriginHandler(home)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.StripPrefix(origin.RESTPrefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Basic "+origin.InProcessAuthB64())
		h.ServeHTTP(w, r)
	})))
	t.Cleanup(srv.Close)

	paired, err := config.LoadFor("laptop")
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.SaveRemote(paired.Directory(), pairing.Remote{Endpoint: srv.URL, Token: "pair-token", Label: "laptop"}); err != nil {
		t.Fatal(err)
	}
	paired.Confluence = origin.PairedConfluenceConfig()
	if err := paired.Save(); err != nil {
		t.Fatal(err)
	}

	db := newMirror(t)
	res, err := RunConfluence(context.Background(), paired, db.DB, Options{Full: true})
	if err != nil {
		t.Fatalf("RunConfluence: %v", err)
	}
	if res.Fetched < 1 {
		t.Fatalf("fetched = %d, want >= 1", res.Fetched)
	}
	d, err := db.PageDetail(context.Background(), created.ID)
	if err != nil || d == nil {
		t.Fatalf("PageDetail(%s): %v %#v", created.ID, err, d)
	}
	if d.Title != "Home wiki note" || d.SpaceKey != origin.DefaultSpaceKey {
		t.Errorf("page = %q in %q", d.Title, d.SpaceKey)
	}
}
