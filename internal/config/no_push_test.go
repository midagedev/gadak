package config

import "testing"

// GDK-711: Web Push was removed. FeatureNames is what PUT / gadak config
// accept; a leftover "push" leaf would store a flag whose endpoints 404.
func TestFeatureNamesDoesNotIncludePush(t *testing.T) {
	for _, name := range FeatureNames {
		if name == "push" {
			t.Fatalf("FeatureNames still contains %q", name)
		}
	}
	if _, ok := SettingByPath("features.push"); ok {
		t.Fatal("features.push is still a catalog path")
	}
}
