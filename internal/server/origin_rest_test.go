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
	if cfg.IsStandalone() {
		t.Fatal("fixture is standalone")
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

func standaloneServer(t *testing.T) (*Handler, *config.Config) {
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
	cfg.Kind = config.KindStandalone
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

func TestOriginRESTStandalonePassesThrough(t *testing.T) {
	h, _ := standaloneServer(t)
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("standalone:standalone"))
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": auth,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("standalone passthrough myself: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOriginRESTStandalonePOSTWithoutOriginAllowed(t *testing.T) {
	// Missing Origin is allowed (CLI). This is the existing browser guard,
	// not new auth — loopback single-user model (decision 0003).
	h, _ := standaloneServer(t)
	req := testRequest(http.MethodPost, origin.RESTPrefix+"/rest/api/3/search/jql", strings.NewReader(`{"jql":"order by created","maxResults":1}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("standalone:standalone")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("CLI POST without Origin must not be forbidden: %s", rec.Body.String())
	}
	if rec.Code == http.StatusNotFound {
		t.Fatalf("standalone POST must not 404: %s", rec.Body.String())
	}
}
