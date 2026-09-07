package skillinstall

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func TestIsDevBuild(t *testing.T) {
	cases := []struct {
		version string
		dev     bool
	}{
		{DevVersion, true},
		{" 0.0.0-dev\n", true},
		{"", true},   // no build information at all is not trustworthy
		{"  ", true}, // ditto
		{"0.20.2", false},
		{"1.0.0-rc.1", false},
		{"v0.19.3", false},
	}
	for _, c := range cases {
		if got := IsDevBuild(c.version); got != c.dev {
			t.Errorf("IsDevBuild(%q) = %v, want %v", c.version, got, c.dev)
		}
		want := SourceRelease
		if c.dev {
			want = SourceDevTree
		}
		if got := SourceFor(c.version); got != want {
			t.Errorf("SourceFor(%q) = %q, want %q", c.version, got, want)
		}
	}
}

func TestReadBuildRevision(t *testing.T) {
	info := func(settings ...debug.BuildSetting) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Settings: settings}, true }
	}
	cases := []struct {
		name string
		read func() (*debug.BuildInfo, bool)
		want string
	}{
		{"no build info", func() (*debug.BuildInfo, bool) { return nil, false }, ""},
		{"no vcs stamp", info(debug.BuildSetting{Key: "GOARCH", Value: "arm64"}), ""},
		{"clean tree", info(
			debug.BuildSetting{Key: "vcs.revision", Value: "ac8e15470d637b9426b343c6e7b8e91d9f71dbee"},
			debug.BuildSetting{Key: "vcs.modified", Value: "false"},
		), "ac8e154"},
		{"dirty tree", info(
			debug.BuildSetting{Key: "vcs.revision", Value: "ac8e15470d637b9426b343c6e7b8e91d9f71dbee"},
			debug.BuildSetting{Key: "vcs.modified", Value: "true"},
		), "ac8e154+"},
		{"short revision is not padded", info(
			debug.BuildSetting{Key: "vcs.revision", Value: "abc"},
		), "abc"},
	}
	for _, c := range cases {
		if got := readBuildRevision(c.read); got != c.want {
			t.Errorf("%s: readBuildRevision = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestWriteReceiptRecordsProvenance — GDK-1531. The receipt is the only thing
// that outlives the process that wrote the skill, so the provenance has to be
// in it: `gadak doctor` reads the file, not the binary that installed it.
func TestWriteReceiptRecordsProvenance(t *testing.T) {
	for _, c := range []struct {
		version string
		want    string
	}{
		{DevVersion, SourceDevTree},
		{"", SourceDevTree},
		{"0.20.2", SourceRelease},
	} {
		dir := t.TempDir()
		if err := WriteReceipt(dir, "deadbeef", c.version); err != nil {
			t.Fatalf("WriteReceipt(%q): %v", c.version, err)
		}
		r, ok := ReadReceipt(dir)
		if !ok {
			t.Fatalf("%q: no receipt read back", c.version)
		}
		if r.Source != c.want {
			t.Errorf("%q: source = %q, want %q", c.version, r.Source, c.want)
		}
		if r.SourceWord() != c.want {
			t.Errorf("%q: SourceWord = %q, want %q", c.version, r.SourceWord(), c.want)
		}
		if r.Revision != BuildRevision() {
			t.Errorf("%q: revision = %q, want %q", c.version, r.Revision, BuildRevision())
		}
	}
}

// TestReceiptSourceWordBackfillsFromVersion — the receipt the incident left
// behind has no `source` field, because the field did not exist yet. Its
// gadak_version already said 0.0.0-dev, so doctor can still name it correctly
// rather than reporting nothing for every copy installed before this release.
func TestReceiptSourceWordBackfillsFromVersion(t *testing.T) {
	cases := []struct {
		receipt Receipt
		want    string
	}{
		{Receipt{GadakVersion: DevVersion}, SourceDevTree},
		{Receipt{GadakVersion: "0.20.2"}, SourceRelease},
		{Receipt{}, ""}, // nothing recorded: say nothing, do not guess
		{Receipt{Source: SourceRelease, GadakVersion: DevVersion}, SourceRelease}, // explicit wins
	}
	for _, c := range cases {
		if got := c.receipt.SourceWord(); got != c.want {
			t.Errorf("%+v: SourceWord = %q, want %q", c.receipt, got, c.want)
		}
	}
}

// TestReceiptFieldsAreOmittedWhenEmpty keeps the receipt readable for the
// human who opens it: a release build with no vcs stamp must not grow an empty
// "revision": "" line.
func TestReceiptFieldsAreOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := WriteReceipt(dir, "deadbeef", "0.20.2"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ReceiptName))
	if err != nil {
		t.Fatal(err)
	}
	if BuildRevision() == "" && bytes.Contains(raw, []byte("revision")) {
		t.Errorf("no revision was stamped, so the field must be absent:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(`"source": "release"`)) {
		t.Errorf("source missing from the receipt:\n%s", raw)
	}
}
