package main

// GDK-493: the staleness trip used to live inside the skill's own body, so
// the generations that needed the sentence most — the stale ones — were
// exactly the ones without it. The binary owns the check now: a read verb
// must say, once per process on stderr, when an installed skill copy exists
// and differs from this build's embedded text. These tests reproduce the
// three-generation incident's states directly: an older gadak-written copy
// (receipt and all), a hand-edited copy, a current copy, and no copy.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/skillinstall"
)

// plantSkillDiffers writes an older gadak-authored generation at the Claude
// destination: the embedded body plus a trailing generation marker, with the
// receipt gadak leaves beside it, so the installer's own classifier would
// call it stale rather than conflict. That is the bootstrap-gap case — the
// copy on disk is behind, and the copy the agent loaded is behinder still.
func plantSkillDiffers(t *testing.T, marker string) string {
	t.Helper()
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	old := append([]byte{}, gadak.SkillMarkdown()...)
	old = append(old, []byte("\n<!-- "+marker+" -->\n")...)
	if err := os.WriteFile(dest, old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := skillinstall.WriteReceipt(filepath.Dir(dest), old, "0.0.0-test"); err != nil {
		t.Fatal(err)
	}
	return dest
}

// resetSkillDiffersLatch returns the process-once latch to "not spoken" for
// this test and puts it back on the way out, so tests here and elsewhere in
// the package cannot consume each other's warning.
func resetSkillDiffersLatch(t *testing.T) {
	t.Helper()
	skillDiffersWarned = false
	t.Cleanup(func() { skillDiffersWarned = false })
}

func TestReadVerbWarnsWhenInstalledSkillDiffers(t *testing.T) {
	plantSkillDiffers(t, "one generation behind")
	sqlDemoHome(t)
	resetSkillDiffersLatch(t)

	out, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 1"})
	})
	if err != nil {
		t.Fatalf("sql with a differing skill: %v\n%s", err, out)
	}
	if !strings.Contains(stderr, "skill file differs from this build") {
		t.Fatalf("a differing installed skill must warn on stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "`gadak skill install`") {
		t.Fatalf("the warning must teach the way out, got %q", stderr)
	}
	if strings.Contains(out, "skill file differs") {
		t.Fatalf("warning leaked to stdout: %q", out)
	}
}

// The threshold is "differs", never "stale": a hand-edited copy (no receipt,
// not one of gadak's digests) differs from this build as surely as an old
// one, and the binary cannot know which side is newer.
func TestReadVerbWarnsOnHandEditedSkillToo(t *testing.T) {
	home := isolateHomeWithClaude(t)
	dest := skillDestUnder(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("---\nname: gadak\n---\nsomeone's own edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sqlDemoHome(t)
	resetSkillDiffersLatch(t)

	_, stderr, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 1"})
	})
	if err != nil {
		t.Fatalf("sql with a hand-edited skill: %v", err)
	}
	if !strings.Contains(stderr, "skill file differs from this build") {
		t.Fatalf("a hand-edited skill must warn too — the threshold is differs, got %q", stderr)
	}
}

func TestReadVerbSilentWhenSkillCurrentOrMissing(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		home := isolateHomeWithClaude(t)
		dest := skillDestUnder(home)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, gadak.SkillMarkdown(), 0o644); err != nil {
			t.Fatal(err)
		}
		sqlDemoHome(t)
		resetSkillDiffersLatch(t)
		_, stderr, err := captureBoth(t, func() error {
			return cmdSQL([]string{"select key from issues limit 1"})
		})
		if err != nil {
			t.Fatalf("sql with a current skill: %v", err)
		}
		if strings.Contains(stderr, "skill file differs") {
			t.Fatalf("a byte-identical skill must not warn, got %q", stderr)
		}
	})
	t.Run("missing", func(t *testing.T) {
		// No skill anywhere: TestMain's isolated home has none, and a user
		// without an installed copy must not be nagged on every read.
		sqlDemoHome(t)
		resetSkillDiffersLatch(t)
		_, stderr, err := captureBoth(t, func() error {
			return cmdSQL([]string{"select key from issues limit 1"})
		})
		if err != nil {
			t.Fatalf("sql with no skill installed: %v", err)
		}
		if strings.Contains(stderr, "skill file differs") {
			t.Fatalf("no installed copy means nothing to compare — silence, got %q", stderr)
		}
	})
}

// The read verbs share one funnel, so the line is once per process however
// many reads a process performs.
func TestSkillDiffersWarnsOncePerProcess(t *testing.T) {
	plantSkillDiffers(t, "one generation behind")
	sqlDemoHome(t)
	resetSkillDiffersLatch(t)
	_, first, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 1"})
	})
	if err != nil {
		t.Fatalf("first sql: %v", err)
	}
	if n := strings.Count(first, "skill file differs"); n != 1 {
		t.Fatalf("first read must warn exactly once, got %d in %q", n, first)
	}
	_, second, err := captureBoth(t, func() error {
		return cmdSQL([]string{"select key from issues limit 1"})
	})
	if err != nil {
		t.Fatalf("second sql: %v", err)
	}
	if n := strings.Count(second, "skill file differs"); n != 0 {
		t.Fatalf("the second read in one process must stay quiet, got %d in %q", n, second)
	}
}
