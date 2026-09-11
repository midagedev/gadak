package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairing"
)

// TestProjectCreatePairedWorkspaceProceeds is the GDK-1793 gate. A paired
// workspace's origin IS the built-in tracker, one machine away, so
// `project create` must ride the serve passthrough like any other write
// instead of refusing with the built-in-only sentence that blames Jira —
// issue writes on the same workspace already worked. The httptest server
// stands in for the home machine's REST passthrough; asserting the request
// path proves the POST actually went there.
func TestProjectCreatePairedWorkspaceProceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	var gotMethod, gotPath string
	var gotBearer bool
	serve := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBearer = strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"10002","key":"WKS"}`))
	}))
	defer serve.Close()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.SaveRemote(cfg.Directory(), pairing.Remote{
		Endpoint: serve.URL,
		Token:    "device-token",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	// The fixture is exactly the reported shape: Kind empty (so
	// HasBuiltInOrigin is false) with the pairing credential present.
	if cfg.HasBuiltInOrigin() {
		t.Fatal("fixture must leave Kind empty — the paired-workspace shape")
	}
	if cfg.OriginType() != config.OriginGadak || cfg.Transport() != config.TransportRemote {
		t.Fatalf("fixture must be a paired workspace: origin=%s transport=%s", cfg.OriginType(), cfg.Transport())
	}

	out, err := capture(t, func() error {
		return cmdProjectCreate([]string{"WKS", "--name", "Workstation"})
	})
	if err != nil {
		t.Fatalf("project create on a paired workspace must proceed, got: %v", err)
	}
	if !strings.Contains(out, "WKS") || !strings.Contains(out, "project created (id 10002)") {
		t.Fatalf("success line missing from %q", out)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/origin/rest/api/3/project" {
		t.Fatalf("POST went %s %s, want POST /api/v1/origin/rest/api/3/project (the serve passthrough)", gotMethod, gotPath)
	}
	if !gotBearer {
		t.Fatal("the passthrough request must carry the pairing bearer")
	}
}

// TestProjectCreateRefusalNamesTheRealOrigin is the other half of GDK-1793:
// the refusal is only for origins that genuinely cannot grow a project, and
// it must name the workspace's actual origin. The old sentence blamed
// "a Jira workspace" unconditionally — a Linear or Jira Server user was
// told their workspace was Jira's.
func TestProjectCreateRefusalNamesTheRealOrigin(t *testing.T) {
	cases := []struct {
		name string
		set  func(*config.Config)
		want string
	}{
		{"jira", func(c *config.Config) {
			c.Kind, c.Site, c.Email, c.Token = config.OriginJira, "https://example.atlassian.net", "a@b.c", "site-token"
		}, config.OriginJira},
		{"jira-server", func(c *config.Config) {
			c.Kind, c.Site, c.Token = config.OriginJiraServer, "https://jira.example.internal", "pat-token"
		}, config.OriginJiraServer},
		{"linear", func(c *config.Config) {
			c.Kind = config.OriginLinear
			c.Linear = &config.LinearConfig{APIKey: "lin_fixture"}
		}, config.OriginLinear},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("GADAK_HOME", home)
			clearCredentialEnv(t)
			config.SetProfile("")
			t.Cleanup(func() { config.SetProfile("") })

			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			tc.set(cfg)
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			if got := cfg.OriginType(); got != tc.want {
				t.Fatalf("fixture origin type = %s, want %s", got, tc.want)
			}

			_, err = capture(t, func() error {
				return cmdProjectCreate([]string{"WKS", "--name", "Workstation"})
			})
			if err == nil {
				t.Fatalf("a %s workspace must refuse project create", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal must name the origin %q, got: %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "on a Jira workspace") && tc.want != config.OriginJira {
				t.Fatalf("refusal blames a Jira workspace this %s workspace is not, got: %v", tc.name, err)
			}
		})
	}
}
