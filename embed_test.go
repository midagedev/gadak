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

// TestSkillMarkdownCoversTheWriteVerbs pins the skill against the CLI it
// describes. A session that loaded this file and then answered "gadak cannot
// create issues" — or invented a REST call — is worse than one with no skill
// at all, because the first failed write costs the trust the reads earned
// (GDK-91). AGENTS.md is the source these verbs come from.
//
// The check is per-verb rather than a byte hash: the skill is prose and will
// keep being edited, and a hash would fail on every wording change while
// saying nothing about coverage.
func TestSkillMarkdownCoversTheWriteVerbs(t *testing.T) {
	skill := SkillMarkdown()
	for _, verb := range []string{
		"gadak create", "gadak attach", "gadak edit",
		"gadak comment", "gadak transition", "gadak assign",
	} {
		if !bytes.Contains(skill, []byte(verb)) {
			t.Errorf("skills/gadak/SKILL.md never mentions %q — a session with this "+
				"skill loaded cannot know the verb exists", verb)
		}
	}
}
