package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GDK-710: catalog entries for leftover fieldMap / editableFields must not
// accept a value that survives to disk. LoadFor still migrates an old file;
// Get stays so `gadak config get fieldMap` on leftover in-memory state works.
func TestLegacyFieldMapSetDoesNotSurviveToDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"fieldMap", "editableFields"} {
		s, ok := SettingByPath(path)
		if !ok {
			t.Fatalf("%s must stay in the catalog so get still works", path)
		}
		if !strings.Contains(s.Description, "legacy") {
			t.Errorf("%s Description must say the key is legacy, got %q", path, s.Description)
		}

		setErr := s.Set(c, json.RawMessage(`{"severity":"customfield_10050"}`))
		if path == "fieldMap" && len(c.FieldMap) != 0 {
			t.Errorf("Set planted in-memory FieldMap: %v", c.FieldMap)
		}
		if path == "editableFields" && len(c.EditableFields) != 0 {
			t.Errorf("Set planted in-memory EditableFields: %v", c.EditableFields)
		}
		if setErr == nil {
			t.Errorf("%s Set must refuse — a planted value is migrated away on the next load", path)
		} else if !strings.Contains(setErr.Error(), "fields") {
			t.Errorf("%s refusal must name the replacement key fields, got %v", path, setErr)
		}
	}

	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("saved config: %v\n%s", err, raw)
	}
	if _, ok := disk["fieldMap"]; ok {
		t.Fatalf("fieldMap survived to disk: %s", raw)
	}
	if _, ok := disk["editableFields"]; ok {
		t.Fatalf("editableFields survived to disk: %s", raw)
	}
}

func TestLegacyFieldMapGetStillReadsLeftover(t *testing.T) {
	fm, ok := SettingByPath("fieldMap")
	if !ok {
		t.Fatal("fieldMap missing from catalog")
	}
	ef, ok := SettingByPath("editableFields")
	if !ok {
		t.Fatal("editableFields missing from catalog")
	}

	empty := &Config{}
	if got := fm.Get(empty); len(got.(map[string]string)) != 0 {
		t.Fatalf("empty Get fieldMap = %#v", got)
	}
	if got := ef.Get(empty); len(got.(map[string]string)) != 0 {
		t.Fatalf("empty Get editableFields = %#v", got)
	}

	leftover := &Config{
		FieldMap:       map[string]string{"severity": "customfield_10050"},
		EditableFields: map[string]string{"solution": "customfield_10092"},
	}
	if got := fm.Get(leftover).(map[string]string)["severity"]; got != "customfield_10050" {
		t.Fatalf("Get leftover fieldMap = %#v", fm.Get(leftover))
	}
	if got := ef.Get(leftover).(map[string]string)["solution"]; got != "customfield_10092" {
		t.Fatalf("Get leftover editableFields = %#v", ef.Get(leftover))
	}
}
