package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Realistic ids from existing config/sync/server tests — do not invent field ids.

func TestLoadForMigratesLegacyOnlyAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	path := filepath.Join(home, "config.json")
	// Legacy-only: the pre-Fields shape still on disk today.
	legacy := `{
  "site": "https://example.atlassian.net",
  "fieldMap": {
    "storyPoints": "customfield_10016"
  },
  "editableFields": {
    "storyPoints": "customfield_10016"
  }
}
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		c, err := LoadFor("")
		if err != nil {
			t.Fatalf("LoadFor: %v", err)
		}
		if len(c.FieldMap) != 0 {
			t.Fatalf("FieldMap still set after migrate: %v", c.FieldMap)
		}
		if len(c.EditableFields) != 0 {
			t.Fatalf("EditableFields still set after migrate: %v", c.EditableFields)
		}
		if len(c.Fields) != 1 {
			t.Fatalf("Fields = %+v", c.Fields)
		}
		got := c.Fields[0]
		want := FieldSpec{
			Alias: "storyPoints",
			Label: "storyPoints",
			IDs:   []string{"customfield_10016"},
			Role:  "facet",
			Auto:  false,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("synthesized spec:\n got %+v\nwant %+v", got, want)
		}
	})
	if !strings.Contains(stderr, "rewrote") {
		t.Fatalf("migration must log that it rewrote the config, got %q", stderr)
	}
	if !strings.Contains(stderr, "fieldMap") || !strings.Contains(stderr, "editableFields") {
		t.Fatalf("log must name the source shape, got %q", stderr)
	}
	if !strings.Contains(stderr, "fields") {
		t.Fatalf("log must name the destination shape, got %q", stderr)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if _, ok := disk["fieldMap"]; ok {
		t.Fatalf("disk still has fieldMap: %s", raw)
	}
	if _, ok := disk["editableFields"]; ok {
		t.Fatalf("disk still has editableFields: %s", raw)
	}
	fi1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	stderr2 := captureStderr(t, func() {
		c2, err := LoadFor("")
		if err != nil {
			t.Fatalf("second LoadFor: %v", err)
		}
		if len(c2.FieldMap) != 0 || len(c2.EditableFields) != 0 {
			t.Fatalf("second load reintroduced legacy keys: %+v", c2)
		}
		if len(c2.Fields) != 1 || c2.Fields[0].Alias != "storyPoints" {
			t.Fatalf("second load Fields = %+v", c2.Fields)
		}
	})
	if stderr2 != "" {
		t.Fatalf("second load must not log a rewrite, got %q", stderr2)
	}
	raw2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw2) != string(raw) {
		t.Fatalf("second load rewrote the file\n first: %s\nsecond: %s", raw, raw2)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatalf("second load churned mtime: %v → %v", fi1.ModTime(), fi2.ModTime())
	}
}

func TestLoadForFieldsOnlyDoesNotRewrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	path := filepath.Join(home, "config.json")
	body := `{
  "fields": [
    {
      "alias": "storyPoints",
      "label": "Story Points",
      "ids": ["customfield_10016"],
      "role": "facet",
      "kind": "option"
    }
  ]
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fi1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		c, err := LoadFor("")
		if err != nil {
			t.Fatalf("LoadFor: %v", err)
		}
		if len(c.Fields) != 1 || c.Fields[0].Alias != "storyPoints" || c.Fields[0].Kind != "option" {
			t.Fatalf("Fields-only load mutated specs: %+v", c.Fields)
		}
		if len(c.FieldMap) != 0 || len(c.EditableFields) != 0 {
			t.Fatalf("legacy keys appeared: fieldMap=%v editable=%v", c.FieldMap, c.EditableFields)
		}
	})
	if stderr != "" {
		t.Fatalf("Fields-only config must not log a rewrite, got %q", stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Fields-only config was rewritten\n before: %s\n after: %s", before, after)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatalf("Fields-only load churned mtime: %v → %v", fi1.ModTime(), fi2.ModTime())
	}
}

func TestLoadForBothShapesUsesFieldSpecsThenEditableOverlay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	path := filepath.Join(home, "config.json")
	// Fields present + leftover FieldMap + conflicting EditableFields.
	// FieldSpecs() keeps Fields (storyPoints in FieldMap is ignored).
	// EditableAliases then lets editableFields win on the shared alias.
	both := `{
  "fields": [
    {
      "alias": "severity",
      "label": "Severity Level",
      "ids": ["customfield_10"],
      "role": "facet",
      "kind": "option"
    }
  ],
  "fieldMap": {
    "storyPoints": "customfield_10016",
    "severity": "customfield_99"
  },
  "editableFields": {
    "severity": "customfield_legacy"
  }
}
`
	if err := os.WriteFile(path, []byte(both), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.FieldMap) != 0 || len(c.EditableFields) != 0 {
		t.Fatalf("legacy keys survived: fieldMap=%v editable=%v", c.FieldMap, c.EditableFields)
	}
	if len(c.Fields) != 1 {
		t.Fatalf("want 1 spec (FieldMap-only alias dropped — FieldSpecs sole-truth), got %+v", c.Fields)
	}
	s := c.Fields[0]
	if s.Alias != "severity" || !reflect.DeepEqual(s.IDs, []string{"customfield_legacy"}) {
		t.Fatalf("legacy editableFields must win the id, got %+v", s)
	}
	if s.Label != "Severity Level" || s.Role != "facet" || s.Kind != "option" {
		t.Fatalf("overlay must keep Label/Role/Kind from Fields, got %+v", s)
	}

	var disk map[string]any
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if _, ok := disk["fieldMap"]; ok {
		t.Fatalf("disk still has fieldMap: %s", raw)
	}
	if _, ok := disk["editableFields"]; ok {
		t.Fatalf("disk still has editableFields: %s", raw)
	}
}

// TestLoadForMigratesLegacyWhenDirReadOnly is GDK-173: a locked home must
// still load. Normalize in memory; a failed rewrite is a warning, not an error.
func TestLoadForMigratesLegacyWhenDirReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	path := filepath.Join(home, "config.json")
	legacy := `{
  "site": "https://example.atlassian.net",
  "fieldMap": {
    "storyPoints": "customfield_10016"
  },
  "editableFields": {
    "storyPoints": "customfield_10016"
  }
}
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) })

	probe := filepath.Join(home, ".write-probe")
	if f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		t.Skip("filesystem still allows write as owner; cannot assert read-only migrate")
	}

	var c *Config
	stderr := captureStderr(t, func() {
		var err error
		c, err = LoadFor("")
		if err != nil {
			t.Fatalf("LoadFor: %v", err)
		}
	})
	if c == nil {
		t.Fatal("LoadFor returned nil config")
	}
	if len(c.FieldMap) != 0 || len(c.EditableFields) != 0 {
		t.Fatalf("callers must not see legacy keys: fieldMap=%v editable=%v", c.FieldMap, c.EditableFields)
	}
	if len(c.Fields) != 1 || c.Fields[0].Alias != "storyPoints" {
		t.Fatalf("in-memory Fields = %+v", c.Fields)
	}
	if strings.Contains(stderr, "rewrote") {
		t.Fatalf("must not claim the rewrite succeeded, got %q", stderr)
	}
	if !strings.Contains(stderr, path) {
		t.Fatalf("warning must name the path %q, got %q", path, stderr)
	}
	if !strings.Contains(stderr, "migrate field mapping") {
		t.Fatalf("warning must be greppable as migrate field mapping, got %q", stderr)
	}
	if !strings.Contains(stderr, "retry") {
		t.Fatalf("warning must say the rewrite will be retried, got %q", stderr)
	}
	if strings.Count(stderr, "\n") != 1 || !strings.HasSuffix(stderr, "\n") {
		t.Fatalf("warning must be one line, got %q", stderr)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("leftover %s.tmp: %v", path, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if _, ok := disk["fieldMap"]; !ok {
		t.Fatalf("read-only load rewrote disk: %s", raw)
	}
}
