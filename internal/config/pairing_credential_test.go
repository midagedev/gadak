package config

import (
	"testing"

	"github.com/midagedev/gadak/internal/pairing"
)

// A paired workspace (GDK-433) stores its credential in remote-origin.json,
// not in config.json — HasCredential is the single owner every verb gates on
// (sync, api, fields, agent writes), so it must count that file too (GDK-442:
// three verbs classified the same workspace three different ways).
func TestHasCredentialPaired(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	c, err := LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	if c.HasCredential() {
		t.Fatal("empty workspace must not report a credential")
	}
	if err := pairing.SaveRemote(c.Directory(), pairing.Remote{
		Endpoint: "https://home.example.com:8443",
		Token:    "device-token",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	if !c.HasCredential() {
		t.Fatal("paired workspace (remote-origin.json) must count as a credential")
	}
}
