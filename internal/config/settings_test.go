package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateTheme(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"system", "", false},
		{"light", "light", false},
		{"dark", "dark", false},
		{"paper", "paper", false},
		{"slate-2", "slate-2", false},
		{"Dark", "", true},
		{"LIGHT", "", true},
		{"has space", "", true},
		{"under_score", "", true},
		{strings.Repeat("a", 33), "", true},
		{strings.Repeat("a", 32), strings.Repeat("a", 32), false},
		{"UPPER", "", true},
		{"slash/no", "", true},
	}
	for _, tc := range cases {
		got, err := ValidateTheme(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ValidateTheme(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateTheme(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ValidateTheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEffectiveThemeEmptyIsSystem(t *testing.T) {
	if got := (*Config)(nil).EffectiveTheme(); got != "system" {
		t.Fatalf("nil: %q", got)
	}
	if got := (&Config{}).EffectiveTheme(); got != "system" {
		t.Fatalf("empty: %q", got)
	}
	if got := (&Config{Appearance: &Appearance{Theme: "dark"}}).EffectiveTheme(); got != "dark" {
		t.Fatalf("dark: %q", got)
	}
}

func TestSettingsCatalogHasRequiredPaths(t *testing.T) {
	paths := map[string]bool{}
	for _, s := range Settings() {
		if s.Path == "" || s.Root == "" || s.Description == "" {
			t.Errorf("incomplete setting %+v", s)
		}
		if s.Get == nil || s.Set == nil {
			t.Errorf("%s missing Get/Set", s.Path)
		}
		if paths[s.Path] {
			t.Errorf("duplicate path %q", s.Path)
		}
		paths[s.Path] = true
	}
	for _, name := range FeatureNames {
		want := "features." + name
		if !paths[want] {
			t.Errorf("catalog missing %q", want)
		}
	}
	for _, want := range []string{
		"appearance.theme",
		"syncIntervalSec",
		"reconcileIntervalSec",
		"staleThresholdHours",
		"projects",
		"notify",
		"updateCheck",
		"devStatus",
	} {
		if !paths[want] {
			t.Errorf("catalog missing %q", want)
		}
	}
}

func TestSettingGetSetAppearanceTheme(t *testing.T) {
	s, ok := SettingByPath("appearance.theme")
	if !ok {
		t.Fatal("appearance.theme not in catalog")
	}
	c := &Config{}
	if got := s.Get(c); got != "system" {
		t.Fatalf("default get = %#v", got)
	}
	if err := s.Set(c, json.RawMessage(`"dark"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance == nil || c.Appearance.Theme != "dark" {
		t.Fatalf("stored %+v", c.Appearance)
	}
	if got := s.Get(c); got != "dark" {
		t.Fatalf("get after set = %#v", got)
	}
	if err := s.Set(c, json.RawMessage(`"system"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance != nil {
		t.Fatalf("system must store nil, got %+v", c.Appearance)
	}
	if err := s.Set(c, json.RawMessage(`"Nope"`)); err == nil {
		t.Fatal("invalid theme accepted")
	}
}

func TestSettingGetSetDevStatus(t *testing.T) {
	s, ok := SettingByPath("devStatus")
	if !ok {
		t.Fatal("devStatus not in catalog")
	}
	if !strings.Contains(s.Description, "dev-status") {
		t.Errorf("description must name Jira's internal dev-status API, got %q", s.Description)
	}
	if !strings.Contains(s.Description, "per-issue") {
		t.Errorf("description must name the per-issue sync cost, got %q", s.Description)
	}
	c := &Config{}
	if got := s.Get(c); got != false {
		t.Fatalf("default get = %#v, want false", got)
	}
	if err := s.Set(c, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if !c.DevStatus {
		t.Fatal("Set(true) did not store DevStatus")
	}
	if got := s.Get(c); got != true {
		t.Fatalf("get after set = %#v", got)
	}
	if err := s.Set(c, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if c.DevStatus {
		t.Fatal("Set(false) did not clear DevStatus")
	}
}

func TestSettingSetIntervalFloor(t *testing.T) {
	s, ok := SettingByPath("syncIntervalSec")
	if !ok {
		t.Fatal("syncIntervalSec missing")
	}
	c := &Config{}
	if err := s.Set(c, json.RawMessage(`5`)); err == nil {
		t.Fatal("below-floor sync accepted")
	}
	if err := s.Set(c, json.RawMessage(`30`)); err != nil {
		t.Fatal(err)
	}
	if c.SyncIntervalSec != 30 {
		t.Fatalf("got %d", c.SyncIntervalSec)
	}
}

func TestValidateIntervalsMoved(t *testing.T) {
	if err := ValidateIntervals(5, 0); err == nil || !strings.Contains(err.Error(), "syncIntervalSec") {
		t.Fatalf("want sync floor error, got %v", err)
	}
	if err := ValidateIntervals(0, 60); err == nil || !strings.Contains(err.Error(), "reconcileIntervalSec") {
		t.Fatalf("want reconcile floor error, got %v", err)
	}
	if err := ValidateIntervals(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIntervals(15, 300); err != nil {
		t.Fatal(err)
	}
}

func TestSettingSetProjectsRejectsInvalidKeys(t *testing.T) {
	// GDK-809: the projects setter is the single choke for key shape.
	// Site membership is CLI-only (needs an origin); this rejects garbage
	// that would otherwise be stored from config set and PUT settings/.
	s, ok := SettingByPath("projects")
	if !ok {
		t.Fatal("projects not in catalog")
	}
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty entry", `[""]`, "project key"},
		{"whitespace", `["  "]`, "project key"},
		{"display-name words", `["Numbers"]`, "project key"},
		{"control char", string(mustJSONStrings(t, "NMB\n")), "project key"},
		{"too long", `["` + strings.Repeat("A", 11) + `"]`, "project key"},
		{"punctuation", `["DI!"]`, "project key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			err := s.Set(c, json.RawMessage(tc.raw))
			if err == nil {
				t.Fatalf("accepted %s", tc.raw)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing %q", err, tc.want)
			}
			if len(c.Projects) != 0 {
				t.Errorf("stored %v after rejection", c.Projects)
			}
		})
	}
}

func TestSettingSetProjectsNormalizesAndAcceptsEmpty(t *testing.T) {
	s, ok := SettingByPath("projects")
	if !ok {
		t.Fatal("projects not in catalog")
	}
	c := &Config{}
	if err := s.Set(c, json.RawMessage(`[]`)); err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if c.Projects == nil || len(c.Projects) != 0 {
		t.Fatalf("empty list stored %+v", c.Projects)
	}
	if err := s.Set(c, json.RawMessage(`["nmb","NMB","d1"]`)); err != nil {
		t.Fatalf("valid keys: %v", err)
	}
	if got := strings.Join(c.Projects, ","); got != "NMB,D1" {
		t.Fatalf("normalized %v", c.Projects)
	}
}

func mustJSONStrings(t *testing.T, keys ...string) []byte {
	t.Helper()
	b, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExistingUnknownProjectsSurviveDecode(t *testing.T) {
	// GDK-809 "손상": a config.json that already contains a site-unknown or
	// ill-shaped key must still Load. Validation is Set-only.
	var c Config
	if err := json.Unmarshal([]byte(`{"projects":["DI","Numbers"]}`), &c); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Projects, ","); got != "DI,Numbers" {
		t.Fatalf("Load-equivalent decode rewrote stored projects: %v", c.Projects)
	}
}

func TestLooksLikeProjectKey(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"GDK", true},
		{"D1", true},
		{"NMB", true},
		{"CRWN", true},
		{"Fix", false}, // help example first word; do not case-fold
		{"FIX", true},
		{"A", false},
		{"", false},
		{strings.Repeat("A", 11), false},
		{"GDK-1", false},
		{"nmb", false},
	}
	for _, tc := range cases {
		if got := LooksLikeProjectKey(tc.in); got != tc.want {
			t.Errorf("LooksLikeProjectKey(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}
