package jira

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func deploymentServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/serverInfo" {
			// What a Server instance answers an absent v3 route with
			// depends on the credential — measured on 11.3.11: 404 to a
			// valid basic login, 401 to a Cloud-shaped one, 302 to a
			// bearer PAT or to none. 401 stands in for the whole set
			// here: the point is that the status says nothing about
			// whether the route exists.
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDeploymentReadsServerInfoNotTheV3StatusCode(t *testing.T) {
	for _, tc := range []struct {
		reported string
		want     string
	}{
		{"Cloud", config.OriginJira},
		{"Server", config.OriginJiraServer},
		{"DataCenter", config.OriginJiraServer},
	} {
		srv := deploymentServer(t, `{"deploymentType":"`+tc.reported+`","version":"11.3.11"}`)
		got, err := NewAnonymous(srv.URL).Deployment(context.Background())
		if err != nil {
			t.Fatalf("deploymentType %q: %v", tc.reported, err)
		}
		if got != tc.want {
			t.Fatalf("deploymentType %q: got %q, want %q", tc.reported, got, tc.want)
		}
	}
}

func TestUnknownDeploymentTypeIsAnErrorNotACloudDefault(t *testing.T) {
	srv := deploymentServer(t, `{"deploymentType":"Mystery"}`)
	got, err := NewAnonymous(srv.URL).Deployment(context.Background())
	if err == nil {
		t.Fatalf("an unknown deploymentType answered %q instead of failing", got)
	}
	if !strings.Contains(err.Error(), "Mystery") {
		t.Fatalf("error does not name what was reported: %v", err)
	}
}

func TestServerClientSendsTheTokenAsABearer(t *testing.T) {
	var got, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, path = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deploymentType":"Server"}`))
	}))
	defer srv.Close()

	// A context path is the common Server deployment shape (the measured
	// instance serves under /jira); the base must survive into the URL.
	if _, err := NewServer(srv.URL+"/jira", "pat-value").Deployment(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/jira/rest/api/2/serverInfo" {
		t.Fatalf("request path = %q — the context path was dropped", path)
	}
	if got != "Bearer pat-value" {
		t.Fatalf("Authorization = %q, want a bearer PAT", got)
	}
}

func TestAnonymousClientSendsNoAuthorizationHeader(t *testing.T) {
	seen := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Header["Authorization"]; ok {
			seen = strings.Join(v, ",")
		} else {
			seen = ""
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deploymentType":"Cloud"}`))
	}))
	defer srv.Close()

	if _, err := NewAnonymous(srv.URL).Deployment(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("anonymous request carried Authorization %q", seen)
	}
}
