package config

// The GDK-1130 ratchet: config.ParseTimestamp is the single owner of the
// mirror/origin timestamp layout table, and any file that still hands two
// or more distinct layouts to time.Parse — the private-table shape four
// parsers drifted in — must be on the pending list below with a reason.
// Folding a file is taking it off the list; growing the list is a conscious
// act this test forces, not something a new copy of the table can do
// quietly. internal/config itself is outside the scan: the owner lives
// here, and parseTokenExpiresAt's date-only-plus-typed-error contract is a
// different domain, not a straggler.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// pendingTimestampLadders is every remaining private table, with why it is
// still here. An entry is a promise to fold or a documented exception —
// never a permanent parking spot.
var pendingTimestampLadders = map[string]string{
	// The calendar's own vocabulary, added beside the owner by design
	// (space-separated instants, date-only) — not a fold candidate.
	"internal/calendar/calendar.go": "calendar's own shapes (space-separated, date-only) beside config.ParseTimestamp",
	// The rest are fold candidates: their ISO subsets are exactly the
	// owner's set, and each keeps a private table only because its round
	// did not reach it.
	"internal/migrate/linear.go":      "RFC3339Nano + jira no-colon — owner covers both",
	"internal/origin/linearwriter.go": "jira no-colon + RFC3339 — owner covers both",
	"internal/retro/retro.go":         "ISO subset + space + date-only — ISO subset folds",
	"internal/sync/confluence.go":     "watermark ladder — exactly the owner's set",
	"internal/sync/sync.go":           "two watermark ladders — exactly the owner's set",
	"internal/store/durations.go":     "ISOMilli + RFC3339 — owner covers both",
	// Two single-layout validators that happen to share a file — a Date
	// check and a DateTime check, not a parse union. No fold.
	"internal/linear/write.go": "validateDueDate (date-only) + validateStamp (RFC3339) — separate validators, not a union ladder",
}

// timeParseArg captures the first argument of a time.Parse call — a layout
// constant, a literal, or the `layout` variable a slice-driven ladder
// parses with.
var timeParseArg = regexp.MustCompile(`time\.Parse\(\s*([^,\n]+),`)

// layoutRange catches the slice-driven ladder shape
// (`for _, layout := range []string{...}` fed to time.Parse): its layouts
// never appear as a parse argument, so the arg-count rule alone would
// bless them.
var layoutRange = regexp.MustCompile(`for _, layout := range \[\]string\{`)

func TestTimestampLaddersSingleOwner(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, top := range []string{filepath.Join(root, "internal"), filepath.Join(root, "cmd")} {
		filepath.WalkDir(top, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// The owner's own package is where the table is allowed to
			// live; tests legitimately pin exact spellings.
			if d.IsDir() {
				if filepath.Base(path) == "config" && filepath.Dir(path) == filepath.Join(root, "internal") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(data)
			distinct := map[string]bool{}
			for _, m := range timeParseArg.FindAllStringSubmatch(src, -1) {
				distinct[strings.TrimSpace(m[1])] = true
			}
			// Two or more distinct parse arguments is the private-table
			// shape. So is the slice-driven ladder: it parses with a loop
			// variable, the scan sees one name instead of the literals, and
			// layoutRange is what says the table exists at all.
			if len(distinct) >= 2 || (len(distinct) > 0 && layoutRange.MatchString(src)) {
				offenders = append(offenders, rel+" ["+strings.Join(sortedKeys(distinct), ", ")+"]")
			}
			return nil
		})
	}
	sort.Strings(offenders)

	var unlisted []string
	for _, o := range offenders {
		file := o[:strings.Index(o, " [")]
		if _, ok := pendingTimestampLadders[file]; !ok {
			unlisted = append(unlisted, o)
		}
	}
	if len(unlisted) > 0 {
		t.Fatalf("new private timestamp layout tables — fold them into config.ParseTimestamp or add them to pendingTimestampLadders with a reason:\n%s",
			strings.Join(unlisted, "\n"))
	}
	// And the reverse ratchet: a pending entry whose file no longer has a
	// table is a stale promise — remove it.
	live := map[string]bool{}
	for _, o := range offenders {
		live[o[:strings.Index(o, " [")]] = true
	}
	for file := range pendingTimestampLadders {
		if !live[file] {
			t.Errorf("pendingTimestampLadders lists %s, which no longer carries a private table — remove the entry", file)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
