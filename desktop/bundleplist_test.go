package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/deeplink"
)

// The scheme axis has one rule: assert the shipped bundle, not the script that
// writes it. LaunchServices reads Contents/Info.plist out of the packed
// Gadak.app — it never reads build-app.sh — so a plist that the script no
// longer produces (a heredoc edited, a pack step reordered, an `plutil`
// rewrite in a signing path) is invisible to a test that greps the script.
//
// Where the bundle comes from:
//   - GADAK_BUNDLE=/path/to/Gadak.app, when a caller has one (CI sets it after
//     extracting the dmg);
//   - otherwise desktop/build/Gadak.app, which desktop/build-app.sh leaves
//     behind, so a local `desktop/build-app.sh` makes this test run for free.
//
// With no bundle anywhere the test skips — a developer without Xcode must not
// be blocked. GADAK_BUNDLE_REQUIRED=1 turns that skip into a failure, and CI
// sets it: the one place a bundle is guaranteed is the one place the skip
// would be a silent hole.
func bundleUnderTest(t *testing.T) string {
	t.Helper()
	required := os.Getenv("GADAK_BUNDLE_REQUIRED") == "1"
	candidates := []string{}
	if p := os.Getenv("GADAK_BUNDLE"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, filepath.Join("build", "Gadak.app"))

	for _, app := range candidates {
		plist := filepath.Join(app, "Contents", "Info.plist")
		if _, err := os.Stat(plist); err == nil {
			return plist
		}
	}
	msg := fmt.Sprintf("no packed bundle found (looked for Contents/Info.plist under %s); "+
		"build one with desktop/build-app.sh, or point GADAK_BUNDLE at an extracted Gadak.app",
		strings.Join(candidates, ", "))
	if required {
		t.Fatalf("GADAK_BUNDLE_REQUIRED=1 but %s", msg)
	}
	t.Skip(msg)
	return ""
}

// parsePlist decodes an XML property list into Go values: <dict> becomes
// map[string]any, <array> []any, the scalar elements strings/bools. Written by
// hand rather than shelling out to plutil so the assertion is the same on a
// Linux runner extracting an artifact as it is on macOS.
func parsePlist(r io.Reader) (any, error) {
	d := xml.NewDecoder(r)
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "plist" {
			continue
		}
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			if s, ok := tok.(xml.StartElement); ok {
				return plistValue(d, s)
			}
			if _, ok := tok.(xml.EndElement); ok {
				return nil, fmt.Errorf("empty <plist>")
			}
		}
	}
}

func plistValue(d *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		out := map[string]any{}
		key := ""
		haveKey := false
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					var s string
					if err := d.DecodeElement(&s, &t); err != nil {
						return nil, err
					}
					key, haveKey = s, true
					continue
				}
				v, err := plistValue(d, t)
				if err != nil {
					return nil, err
				}
				if !haveKey {
					return nil, fmt.Errorf("<%s> in dict with no preceding <key>", t.Name.Local)
				}
				out[key] = v
				haveKey = false
			case xml.EndElement:
				return out, nil
			}
		}
	case "array":
		out := []any{}
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := plistValue(d, t)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			case xml.EndElement:
				return out, nil
			}
		}
	case "true", "false":
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	default: // string, integer, real, date, data
		var s string
		if err := d.DecodeElement(&s, &start); err != nil {
			return nil, err
		}
		return s, nil
	}
}

// TestPackedBundleRegistersTheScheme is the real cross-artifact check: it
// parses the Info.plist inside the bundle that ships and asserts the scheme
// the handler answers to is registered there. A disagreement between the two
// halves produces no error anywhere at runtime — macOS simply never delivers
// the URL and the link does nothing — so this is the only place it surfaces.
func TestPackedBundleRegistersTheScheme(t *testing.T) {
	path := bundleUnderTest(t)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	v, err := parsePlist(f)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	root, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: root is %T, want a dict", path, v)
	}

	types, ok := root["CFBundleURLTypes"].([]any)
	if !ok || len(types) == 0 {
		t.Fatalf("%s declares no CFBundleURLTypes: the OS will never deliver a %s:// URL",
			path, deeplink.Scheme)
	}
	got := []string{}
	for _, entry := range types {
		dict, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		schemes, _ := dict["CFBundleURLSchemes"].([]any)
		for _, s := range schemes {
			if str, ok := s.(string); ok {
				got = append(got, str)
			}
		}
	}
	for _, s := range got {
		if s == deeplink.Scheme {
			return
		}
	}
	t.Fatalf("%s registers schemes %v, which does not include %q: a %s:// link will not reach this app",
		path, got, deeplink.Scheme, deeplink.Scheme)
}
