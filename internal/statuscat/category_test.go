package statuscat

import "testing"

func TestCategoryNormalizesJiraKeys(t *testing.T) {
	for key, want := range map[string]string{
		"new": "new", "indeterminate": "inprogress", "inprogress": "inprogress", "done": "done", "undefined": "new", "": "new",
	} {
		if got := Category(key); got != want {
			t.Errorf("Category(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestKnownCategoryRefusesUnknown(t *testing.T) {
	if cat, ok := KnownCategory("undefined"); ok {
		t.Errorf("KnownCategory(undefined) = %q, true; want false", cat)
	}
	if cat, ok := KnownCategory(""); ok {
		t.Errorf("KnownCategory(empty) = %q, true; want false", cat)
	}
	if cat, ok := KnownCategory("indeterminate"); !ok || cat != "inprogress" {
		t.Errorf("KnownCategory(indeterminate) = %q %v, want inprogress true", cat, ok)
	}
}

func TestCategoryKeyRoundTrip(t *testing.T) {
	for _, key := range []string{"new", "indeterminate", "inprogress", "done"} {
		if got := CategoryKey(Category(key)); got != CategoryKey(key) {
			t.Errorf("CategoryKey(Category(%q)) = %q, CategoryKey(%q) = %q", key, got, key, CategoryKey(key))
		}
	}
	if got := CategoryKey("inprogress"); got != "indeterminate" {
		t.Errorf("CategoryKey(inprogress) = %q, want indeterminate", got)
	}
	if got := Category(CategoryKey("inprogress")); got != "inprogress" {
		t.Errorf("Category(CategoryKey(inprogress)) = %q, want inprogress", got)
	}
}
