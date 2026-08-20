package sync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// frozenJSON is the on-disk shape a demo/recording workspace writes. Tests
// unmarshal it rather than constructing Config.Frozen by name so a missing
// field is an assertion failure (JSON drop) instead of a compile failure.
const frozenJSON = `{
	"frozen": true,
	"site": "http://127.0.0.1:1",
	"email": "a@example.invalid",
	"token": "tok",
	"confluence": {},
	"linear": {"apiKey": "lin"}
}`

func loadFrozenCfg(t *testing.T) *config.Config {
	t.Helper()
	var cfg config.Config
	if err := json.Unmarshal([]byte(frozenJSON), &cfg); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

// TestFrozenEntryPointsRefuseBeforeNetwork is GDK-181: every exported pull
// must return ErrFrozen with no network and no store write. 127.0.0.1:1 is
// connection-refused immediately — if the gate is missing, the error is a
// transport failure, not ErrFrozen.
func TestFrozenEntryPointsRefuseBeforeNetwork(t *testing.T) {
	cfg := loadFrozenCfg(t)
	if !cfg.SyncFrozen() {
		t.Fatal("unmarshaled config is not frozen — Frozen field missing or json tag wrong")
	}
	db := newMirror(t)
	ctx := context.Background()

	t.Run("Run", func(t *testing.T) {
		_, err := Run(ctx, cfg, db.DB, Options{})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("Run: %v, want ErrFrozen", err)
		}
	})
	t.Run("RunConfluence", func(t *testing.T) {
		_, err := RunConfluence(ctx, cfg, db.DB, Options{})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("RunConfluence: %v, want ErrFrozen", err)
		}
	})
	t.Run("RunLinear", func(t *testing.T) {
		_, err := RunLinear(ctx, cfg, db.DB, Options{})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("RunLinear: %v, want ErrFrozen", err)
		}
	})
	t.Run("Watch", func(t *testing.T) {
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		err := Watch(wctx, cfg, db.DB, Options{Tick: 20 * time.Millisecond})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("Watch: %v, want ErrFrozen", err)
		}
	})
}

// TestUnfrozenCfgDoesNotReturnErrFrozen: the gate is a no-op when frozen is
// off. Missing credential is still the existing error (TestRunRejectsMissingConfig).
func TestUnfrozenCfgDoesNotReturnErrFrozen(t *testing.T) {
	db := newMirror(t)
	_, err := Run(context.Background(), &config.Config{}, db.DB, Options{})
	if err == nil {
		t.Fatal("empty cfg: want credential error")
	}
	if errors.Is(err, ErrFrozen) {
		t.Fatalf("unfrozen cfg returned ErrFrozen: %v", err)
	}
	if !strings.Contains(err.Error(), "site, email and token are required") {
		t.Fatalf("unfrozen empty cfg error changed: %v", err)
	}
}
