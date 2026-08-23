package linear

import "testing"

func TestStatusCategoryKnownTypes(t *testing.T) {
	want := map[string]string{
		"started":   "inprogress",
		"unstarted": "new",
		"backlog":   "new",
		"triage":    "new",
		"completed": "done",
		"canceled":  "done",
		"duplicate": "done",
	}
	for typ, cat := range want {
		got, ok := StatusCategory(typ)
		if !ok || got != cat {
			t.Errorf("StatusCategory(%q) = %q, %v; want %q, true", typ, got, ok, cat)
		}
	}
}

func TestStatusCategoryUnknownIsNewNotOk(t *testing.T) {
	got, ok := StatusCategory("sometype-from-2030")
	if ok {
		t.Fatal("unknown type must report ok=false")
	}
	if got != "new" {
		t.Errorf("unknown = %q, want new", got)
	}
}
