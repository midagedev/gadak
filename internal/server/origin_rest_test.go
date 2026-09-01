package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

func TestOriginRESTConnectedIs404(t *testing.T) {
	db, cfg := fixture(t)
	if cfg.HasLocalOrigin() {
		t.Fatal("fixture is local-origin")
	}
	h := New(db, cfg)
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("connected passthrough: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("body %s, want not_found", rec.Body.String())
	}
}

func localOriginServer(t *testing.T) (*Handler, *config.Config) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)
	t.Cleanup(func() {
		_ = h.Close()
		_ = db.Close()
	})
	return h, cfg
}

func TestOriginRESTLocalOriginPassesThrough(t *testing.T) {
	h, _ := localOriginServer(t)
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("local-origin:local-origin"))
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": auth,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("local-origin passthrough myself: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOriginRESTLocalOriginPOSTWithoutOriginAllowed(t *testing.T) {
	// Missing Origin is allowed (CLI). This is the existing browser guard,
	// not new auth — loopback single-user model (decision 0003).
	h, _ := localOriginServer(t)
	req := testRequest(http.MethodPost, origin.RESTPrefix+"/rest/api/3/search/jql", strings.NewReader(`{"jql":"order by created","maxResults":1}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("local-origin:local-origin")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("CLI POST without Origin must not be forbidden: %s", rec.Body.String())
	}
	if rec.Code == http.StatusNotFound {
		t.Fatalf("local-origin POST must not 404: %s", rec.Body.String())
	}
}
