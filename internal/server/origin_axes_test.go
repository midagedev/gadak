package server

// GDK-1280: the served document has to answer both axes on its own, because
// the surface that reads it must never infer either. The case that forced
// the split is a paired workspace — workspaceKind calls it "connected"
// while its origin is gadak's own tracker one machine away — so that is the
// case pinned here.

import (
	"encoding/json"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairing"
)

// webConfig is the document web/src/lib/config.ts loads as config.json.
func settingsAxes(t *testing.T, cfg *config.Config) (kind, origin, transport string) {
	t.Helper()
	doc := webConfig(cfg)
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		WorkspaceKind string `json:"workspaceKind"`
		OriginType    string `json:"originType"`
		Transport     string `json:"transport"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	return got.WorkspaceKind, got.OriginType, got.Transport
}

func TestServedConfigCarriesBothAxes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	cfg, err := config.LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	kind, origin, transport := settingsAxes(t, cfg)
	if kind != config.KindStandalone {
		t.Errorf("in-process: workspaceKind = %q, want %q", kind, config.KindStandalone)
	}
	if origin != config.OriginGadak || transport != config.TransportLocal {
		t.Errorf("in-process: originType/transport = %q/%q, want %q/%q",
			origin, transport, config.OriginGadak, config.TransportLocal)
	}

	// Same origin type, other side of a serve API.
	paired, err := config.LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.SaveRemote(paired.Directory(), pairing.Remote{
		Endpoint: "https://home.example.com:8443",
		Token:    "device-token",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	kind, origin, transport = settingsAxes(t, paired)
	if kind != config.KindConnected {
		t.Errorf("paired: workspaceKind = %q, want the unchanged %q", kind, config.KindConnected)
	}
	if origin != config.OriginGadak {
		t.Errorf("paired: originType = %q, want %q — the old kind is exactly what this fixes",
			origin, config.OriginGadak)
	}
	if transport != config.TransportRemote {
		t.Errorf("paired: transport = %q, want %q", transport, config.TransportRemote)
	}
}
