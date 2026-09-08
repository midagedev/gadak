package atlhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A 200 carrying a web page is not an API answer (GDK-1648). Measured:
// Jira Server redirects a route the credential cannot reach to /login.jsp,
// Go follows it, and the login page's HTML arrives as the response.
func TestAWebPageIsNotAnAPIAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/serverInfo" {
			http.Redirect(w, r, "/login.jsp", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html;charset=UTF-8")
		_, _ = w.Write([]byte("<!DOCTYPE html><html><title>Log in</title></html>"))
	}))
	defer srv.Close()

	cfg := Config{Base: srv.URL, HTTP: srv.Client(), Retries: 1, ErrPrefix: "jira"}
	_, _, err := DoRaw(context.Background(), cfg, http.MethodGet, "/rest/api/3/serverInfo", nil, false, false)
	if !errors.Is(err, ErrNotAPI) {
		t.Fatalf("a login page was accepted as the API's answer: %v", err)
	}
}

// An error page is left alone: the status already says what happened, and
// swallowing the body would lose the origin's own explanation.
func TestAnHTMLErrorPageKeepsItsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<html>boom</html>"))
	}))
	defer srv.Close()

	cfg := Config{Base: srv.URL, HTTP: srv.Client(), Retries: 1, ErrPrefix: "jira"}
	code, body, err := DoRaw(context.Background(), cfg, http.MethodGet, "/rest/api/2/whatever", nil, false, false)
	if err != nil {
		t.Fatalf("an HTML error page became a transport error: %v", err)
	}
	if code != http.StatusInternalServerError || len(body) == 0 {
		t.Fatalf("status %d, %d body bytes — the origin's own explanation was lost", code, len(body))
	}
}
