package origin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairing"
)

// TestEmbeddedSnapshotRefusalsAndYAML is the GDK-768 gate: the seed YAML
// format gadak has always been able to import (the legacy origin/issuetap.yaml
// NewEmbedded re-seeds from) gains its export half — issuetap's Snapshot,
// which had zero callers, behind a gate that keeps every non-local origin out.
//
// The paired refusal is the load-bearing one: a paired workspace is also
// OriginGadak, but constructing the embedded origin on a paired client would
// mint a fresh empty persist on the wrong machine — the quietly-wrong-origin
// class the pairing contract exists to prevent. Transport, not
// HasBuiltInOrigin, is the gate for exactly that reason.
func TestEmbeddedSnapshotRefusalsAndYAML(t *testing.T) {
	if _, err := EmbeddedSnapshot(nil); err == nil || !strings.Contains(err.Error(), "nil config") {
		t.Fatalf("nil cfg: %v, want nil-config error", err)
	}

	// Jira-shaped: the YAML is the built-in tracker's own format, not an
	// export of some other origin's data.
	if _, err := EmbeddedSnapshot(&config.Config{}); !errors.Is(err, ErrExportRefused) {
		t.Fatalf("jira cfg: %v, want ErrExportRefused", err)
	}

	home := t.TempDir()
	cfg := builtInCfg(t, home)

	// Paired: Kind standalone, but the origin runs on another machine.
	if err := pairing.SaveRemote(cfg.Directory(), pairing.Remote{
		Endpoint: "https://home.example.com:8443",
		Token:    "device-token",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := EmbeddedSnapshot(cfg)
	if !errors.Is(err, ErrExportRefused) || !strings.Contains(err.Error(), "paired") {
		t.Fatalf("paired cfg: %v, want ErrExportRefused naming the pairing", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "origin", "issuetap.db")); !os.IsNotExist(statErr) {
		t.Fatalf("paired refusal must not construct the embedded origin; stat=%v", statErr)
	}
	if err := os.Remove(filepath.Join(cfg.Directory(), "remote-origin.json")); err != nil {
		t.Fatal(err)
	}

	// Unpaired built-in: the seed YAML, straight from the live session the
	// rest of the process uses. A fresh workspace exports its seeded shape.
	raw, err := EmbeddedSnapshot(cfg)
	if err != nil {
		t.Fatalf("standalone snapshot: %v", err)
	}
	if !strings.Contains(string(raw), "projects:") {
		t.Fatalf("snapshot is not the seed YAML (%d bytes):\n%.400s", len(raw), raw)
	}
	if _, err := os.Stat(filepath.Join(home, "origin", "issuetap.db")); err != nil {
		t.Fatalf("persist after snapshot: %v", err)
	}
}
