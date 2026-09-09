package sync

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/store"
)

// The write-through-tail owner walk that used to live here is a source lint
// and now lives behind the sourcelint tag in refresh_owner_walk_test.go —
// run it with `bash tools/sourcelint.sh` (GDK-1144).

func TestRefreshIssueRoutesLinear(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	var db *store.DB
	lc := &linear.Client{}
	var gotCfg *config.Config
	var gotDB *store.DB
	var gotClient *linear.Client
	var gotKey string
	var issueN int
	err := refreshIssue(ctx, cfg, db, "LIN-1", LinearSourceID, refreshPaths{
		linear: func(c *config.Config) (*linear.Client, error) {
			gotCfg = c
			return lc, nil
		},
		syncLinear: func(_ context.Context, d *store.DB, c *linear.Client, key string) error {
			gotDB = d
			gotClient = c
			gotKey = key
			return nil
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			issueN++
			return errors.New("jira path must not run")
		},
	})
	if err != nil {
		t.Fatalf("linear path: %v", err)
	}
	if gotCfg != cfg {
		t.Fatal("linear factory must receive the same cfg")
	}
	if gotDB != db || gotClient != lc || gotKey != "LIN-1" {
		t.Fatalf("syncLinear db=%v client=%v key=%q", gotDB, gotClient, gotKey)
	}
	if issueN != 0 {
		t.Fatalf("SyncIssue calls %d, want 0", issueN)
	}
}

func TestRefreshIssueRoutesNonLinear(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	var db *store.DB
	for _, src := range []string{"", "jira", "LINEAR"} {
		var gotCfg *config.Config
		var gotDB *store.DB
		var gotKey string
		var gotOpts Options
		var linearN int
		err := refreshIssue(ctx, cfg, db, "NMB-1", src, refreshPaths{
			linear: func(*config.Config) (*linear.Client, error) {
				linearN++
				return nil, errors.New("linear factory must not run")
			},
			syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
				return errors.New("linear path must not run")
			},
			syncIssue: func(_ context.Context, c *config.Config, d *store.DB, key string, opts Options) error {
				gotCfg = c
				gotDB = d
				gotKey = key
				gotOpts = opts
				return nil
			},
		})
		if err != nil {
			t.Fatalf("src %q: %v", src, err)
		}
		if linearN != 0 {
			t.Fatalf("src %q: linear factory calls %d, want 0", src, linearN)
		}
		if gotCfg != cfg || gotDB != db || gotKey != "NMB-1" {
			t.Fatalf("src %q: cfg/db/key mismatch", src)
		}
		if !reflect.DeepEqual(gotOpts, Options{}) {
			t.Fatalf("src %q: Options = %+v, want zero", src, gotOpts)
		}
	}
}

func TestRefreshIssuePassesErrorsThrough(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	want := errors.New("origin: linear is not configured")
	err := refreshIssue(ctx, cfg, nil, "LIN-1", LinearSourceID, refreshPaths{
		linear: func(*config.Config) (*linear.Client, error) {
			return nil, want
		},
		syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
			return errors.New("syncLinear must not run after factory error")
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			return errors.New("jira path must not run")
		},
	})
	if err != want {
		t.Fatalf("linear factory err = %v, want passthrough", err)
	}

	want = errors.New("linear re-read failed")
	err = refreshIssue(ctx, cfg, nil, "LIN-1", LinearSourceID, refreshPaths{
		linear: func(*config.Config) (*linear.Client, error) {
			return &linear.Client{}, nil
		},
		syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
			return want
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			return errors.New("jira path must not run")
		},
	})
	if err != want {
		t.Fatalf("syncLinear err = %v, want passthrough", err)
	}

	want = errors.New("sync: site, email and token are required")
	err = refreshIssue(ctx, cfg, nil, "NMB-1", "", refreshPaths{
		linear: func(*config.Config) (*linear.Client, error) {
			return nil, errors.New("linear factory must not run")
		},
		syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
			return errors.New("linear path must not run")
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			return want
		},
	})
	if err != want {
		t.Fatalf("syncIssue err = %v, want passthrough", err)
	}
}

func TestRefreshIssuePublicWiresRealPaths(t *testing.T) {
	ctx := context.Background()
	err := RefreshIssue(ctx, &config.Config{}, nil, "LIN-1", LinearSourceID)
	if err == nil || err.Error() != "origin: linear is not configured" {
		t.Fatalf("linear src without credential: %v", err)
	}
	err = RefreshIssue(ctx, &config.Config{}, nil, "NMB-1", "")
	if err == nil || err.Error() != "sync: site, email and token are required" {
		t.Fatalf("jira src without credential: %v", err)
	}
}

func TestRefreshIssuePublicWrapsMirrorStale(t *testing.T) {
	ctx := context.Background()
	err := RefreshIssue(ctx, &config.Config{}, nil, "LIN-1", LinearSourceID)
	if !errors.Is(err, ErrMirrorStale) {
		t.Fatalf("linear factory err must be ErrMirrorStale: %v", err)
	}
	if err.Error() != "origin: linear is not configured" {
		t.Fatalf("Error() must stay the inner sentence: %v", err)
	}
	err = RefreshIssue(ctx, &config.Config{}, nil, "NMB-1", "")
	if !errors.Is(err, ErrMirrorStale) {
		t.Fatalf("syncIssue err must be ErrMirrorStale: %v", err)
	}
	if err.Error() != "sync: site, email and token are required" {
		t.Fatalf("Error() must stay the inner sentence: %v", err)
	}
}
