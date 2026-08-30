package adf

import "testing"

// GDK-1178: jira.Doc now emits codeBlock for a markdown fence, so a body it
// wrote must still be plain-text replaceable — otherwise the second
// `gadak edit -m` on the same issue refuses what the first one created.
func TestCodeBlockIsSimple(t *testing.T) {
	raw := `{"type":"doc","version":1,"content":[{"type":"codeBlock","attrs":{"language":"sh"},"content":[{"type":"text","text":"ls"}]}]}`
	if !IsSimple(raw) {
		t.Fatal("codeBlock must be simple")
	}
	if loss := FormatLoss(raw); len(loss) != 0 {
		t.Fatalf("FormatLoss = %v", loss)
	}
}
