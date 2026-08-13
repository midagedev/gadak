package gadak

import (
	"bytes"
	"os"
	"testing"
)

// TestSkillMarkdownMatchesRepo asserts the binary embed stays byte-identical
// to the source file agents and docs refer to. `gadak skill install` writes
// this content; a drift would mean brew users get a different skill than git.
func TestSkillMarkdownMatchesRepo(t *testing.T) {
	want, err := os.ReadFile("skills/gadak/SKILL.md")
	if err != nil {
		t.Fatalf("read skills/gadak/SKILL.md: %v", err)
	}
	got := SkillMarkdown()
	if len(got) == 0 {
		t.Fatal("SkillMarkdown() is empty — embed failed")
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("embedded skill differs from skills/gadak/SKILL.md (%d vs %d bytes)",
			len(got), len(want))
	}
}
