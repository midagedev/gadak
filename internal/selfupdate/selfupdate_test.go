package selfupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.3.0", "0.3.1", true},
		{"0.3.0", "0.3.0", false},
		{"0.3.1", "0.3.0", false},
		{"0.0.0-dev", "0.3.1", false},
		{"0.0.0-dev", "1.0.0", false},
		{"0.9", "0.10", true},
		{"0.10", "0.9", false},
		{"1.0", "1.0.0", false},
		{"1.0.0", "1.0", false},
		{"1.0.0", "1.0.1", true},
		{"v0.2.0", "v0.3.0", true},
		{"0.3.0-rc1", "0.3.1", false},
		{"0.3.0", "0.3.1-rc1", false},
	}
	for _, tc := range cases {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestCheck_networkThenCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/releases/latest" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query must be empty, got %q", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header must not be set")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.3.1",
			"html_url": "https://github.com/midagedev/scry/releases/tag/v0.3.1",
		})
	}))
	t.Cleanup(srv.Close)

	prev := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = prev })

	dir := t.TempDir()
	ctx := context.Background()

	info, ok := Check(ctx, dir, "0.3.0", true)
	if !ok {
		t.Fatal("first Check: ok=false")
	}
	if info.Latest != "0.3.1" {
		t.Fatalf("latest %q", info.Latest)
	}
	if info.URL == "" {
		t.Fatal("url empty")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits after first = %d", hits.Load())
	}
	cache := filepath.Join(dir, cacheName)
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("cache not created: %v", err)
	}

	info2, ok := Check(ctx, dir, "0.3.0", true)
	if !ok {
		t.Fatal("second Check: ok=false")
	}
	if info2.Latest != info.Latest {
		t.Fatalf("cached latest %q vs %q", info2.Latest, info.Latest)
	}
	if hits.Load() != 1 {
		t.Fatalf("second Check must not hit network; hits=%d", hits.Load())
	}
}

func TestCheck_disabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	prev := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = prev })

	dir := t.TempDir()
	_, ok := Check(context.Background(), dir, "0.3.0", false)
	if ok {
		t.Fatal("enabled=false should return ok=false")
	}
	if hits.Load() != 0 {
		t.Fatalf("network hits=%d", hits.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, cacheName)); !os.IsNotExist(err) {
		t.Fatalf("cache must not exist, err=%v", err)
	}
}

func TestCheck_devVersion(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	prev := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = prev })

	dir := t.TempDir()
	_, ok := Check(context.Background(), dir, "0.0.0-dev", true)
	if ok {
		t.Fatal("dev version should not check")
	}
	if hits.Load() != 0 {
		t.Fatalf("network hits=%d", hits.Load())
	}
}

func TestCheck_serverError(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	prev := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = prev })

	dir := t.TempDir()
	_, ok := Check(context.Background(), dir, "0.3.0", true)
	if ok {
		t.Fatal("500 should return ok=false silently")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
	// Stale/empty cache is fine; no panic, no ok.
}

func TestCheck_staleCacheRefetches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.4.0",
			"html_url": "https://github.com/midagedev/scry/releases/tag/v0.4.0",
		})
	}))
	t.Cleanup(srv.Close)
	prev := APIBase
	APIBase = srv.URL
	t.Cleanup(func() { APIBase = prev })

	dir := t.TempDir()
	stale := Info{
		Latest:    "0.3.0",
		URL:       "https://example.invalid/old",
		CheckedAt: time.Now().UTC().Add(-25 * time.Hour).Format(time.RFC3339),
	}
	if err := writeCache(dir, stale); err != nil {
		t.Fatal(err)
	}
	info, ok := Check(context.Background(), dir, "0.3.0", true)
	if !ok {
		t.Fatal("expected refetch ok")
	}
	if info.Latest != "0.4.0" {
		t.Fatalf("latest %q", info.Latest)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
}
