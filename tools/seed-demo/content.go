package main

import (
	"math/rand"
)

// Deterministic content pools for procedural generation. Map iteration is not
// used for weighted picks so --seed stays reproducible.

type projectProfile struct {
	components  []string
	versions    []string
	typeWeights []weight
	areas       []string
}

type weight struct {
	key    string
	weight int
}

var projectProfiles = map[string]projectProfile{
	"NMB": {
		components: []string{"Dashboard", "Editor", "Billing", "Auth", "Notifications"},
		versions:   []string{"2026.6.0", "2026.7.0", "2026.8.0", "2026.9.0"},
		typeWeights: []weight{
			{"Bug", 45}, {"Story", 30}, {"Task", 25},
		},
		areas: []string{
			"dashboard filter chips", "editor autosave", "billing plan switcher",
			"SSO callback", "notification digest", "keyboard shortcuts",
			"dark mode contrast", "workspace switcher", "CSV export",
			"onboarding checklist", "saved view sharing", "column resizing",
			"inline comment editor", "avatar upload", "seat management table",
			"invoice download", "trial banner", "member invite flow",
			"activity sidebar", "global search box", "bulk selection toolbar",
			"drag-and-drop reordering", "print stylesheet", "session timeout dialog",
			"empty state illustrations", "breadcrumb navigation", "tag autocomplete",
			"date range picker", "attachment preview", "undo toast",
		},
	},
	"NMA": {
		components: []string{"REST API", "Webhooks", "Workers", "SDK", "Rate Limiting"},
		versions:   []string{"v2.3", "v2.4", "v2.5"},
		typeWeights: []weight{
			{"Bug", 40}, {"Story", 25}, {"Task", 35},
		},
		areas: []string{
			"pagination cursor",
			"webhook retry backoff",
			"idempotency key handling",
			"bulk import worker",
			"rate limit headers",
			"OpenAPI schema",
			"token refresh",
			"audit log ingestion",
			"search endpoint",
			"batch delete",
		},
	},
	"NMS": {
		components: []string{"Triage", "Escalation", "Billing Questions"},
		versions:   []string{"Sprint 41", "Sprint 42", "Sprint 43"},
		typeWeights: []weight{
			{"Bug", 70}, {"Task", 30},
		},
		areas: []string{
			"customer cannot log in",
			"invoice shows wrong currency",
			"export never finishes",
			"duplicate notification emails",
			"workspace invite bounces",
			"attachment preview blank",
			"timezone off by one day",
			"search returns nothing",
			"mobile layout broken",
			"seat count mismatch",
		},
	},
}

var bugPatterns = []string{
	"{area} silently fails on first load",
	"{area} throws 500 when the workspace has no members",
	"{area} loses state after a browser refresh",
	"{area} ignores the selected timezone",
	"{area} double-fires on slow connections",
	"{area} renders stale data after an update",
	"{area} breaks for accounts with more than 1000 records",
	"{area} returns results in a nondeterministic order",
}

var storyPatterns = []string{
	"Let users pin {area} to the sidebar",
	"Add keyboard-only navigation to {area}",
	"Show inline validation errors in {area}",
	"Support bulk actions in {area}",
	"Remember the last used {area} per workspace",
}

var taskPatterns = []string{
	"Add integration tests for {area}",
	"Instrument {area} with structured logs",
	"Document {area} in the public reference",
	"Remove the legacy code path behind {area}",
	"Backfill missing rows for {area}",
}

var patterns = map[string][]string{
	"Bug":   bugPatterns,
	"Story": storyPatterns,
	"Task":  taskPatterns,
}

var labelPool = []string{
	"regression", "customer-reported", "needs-repro", "quick-win", "flaky",
	"performance", "accessibility", "security", "tech-debt", "docs",
}

var environments = []string{
	"Chrome 140 / macOS 15",
	"Safari 18 / iOS 18",
	"Firefox 138 / Windows 11",
	"API v2.4 / staging",
	"Production, EU region",
}

var priorityWeights = []weight{
	{"Highest", 6}, {"High", 18}, {"Medium", 46}, {"Low", 22}, {"Lowest", 8},
}

var commentPool = []string{
	"Reproduced on staging with a fresh workspace. Attaching the request id.",
	"This started after the pagination change landed last week.",
	"Not reproducible with a single member — needs at least three.",
	"Workaround for now: reload the page after switching workspaces.",
	"Root cause is the cache key missing the workspace id.",
	"Moving to review, the fix is behind a flag.",
	"Customer confirmed the issue is gone after the deploy.",
	"Reopening — the same trace showed up again this morning.",
	"Deferring: the affected code path is being replaced next quarter.",
	"Added a regression test so this cannot come back silently.",
}

var linkTypes = []string{"Relates", "Blocks", "Duplicate"}

var wantedTypes = []string{"Bug", "Story", "Task"}

func weightedPick(rng *rand.Rand, pool []weight) string {
	total := 0
	for _, w := range pool {
		total += w.weight
	}
	if total <= 0 {
		return pool[0].key
	}
	n := rng.Intn(total)
	for _, w := range pool {
		n -= w.weight
		if n < 0 {
			return w.key
		}
	}
	return pool[len(pool)-1].key
}

func adf(paragraphs []string) map[string]any {
	content := make([]any, 0, len(paragraphs))
	for _, p := range paragraphs {
		content = append(content, map[string]any{
			"type": "paragraph",
			"content": []any{
				map[string]any{"type": "text", "text": p},
			},
		})
	}
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}

func bugDescription(area string) map[string]any {
	return adf([]string{
		"Steps to reproduce: open " + area + " with a workspace that has more than one member.",
		"Expected: the view keeps the selected filters after a reload.",
		"Actual: filters reset to the default and the row count changes.",
		"Impact: reported by two customers this week; no data loss observed.",
	})
}

func plainDescription(summary string) map[string]any {
	return adf([]string{
		"Context: " + summary + ".",
		"Acceptance: behaviour is covered by a test and documented in the reference.",
	})
}

func formatArea(pattern, area string) string {
	// Patterns use the Python "{area}" placeholder.
	out := make([]byte, 0, len(pattern)+len(area))
	for i := 0; i < len(pattern); {
		if i+6 <= len(pattern) && pattern[i:i+6] == "{area}" {
			out = append(out, area...)
			i += 6
			continue
		}
		out = append(out, pattern[i])
		i++
	}
	return string(out)
}
