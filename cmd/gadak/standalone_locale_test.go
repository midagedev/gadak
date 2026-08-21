package main

// GDK-597: a standalone workspace speaks the user's language, and display
// names are never keys. This pins the whole chain at the CLI level — config
// locale → embedded origin (Cloud fidelity: priority names English) →
// locale-triggered mirror rebuild → queries keyed by status_category.

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestStandaloneLocaleRebuildRoundtrip walks the lifecycle:
// English mirror → config set locale ko → create through the recorded type
// ID (not a display name) → sync announces the rebuild and refetches every
// row in Korean → the query keys on status_category, never on a name.
func TestStandaloneLocaleRebuildRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	}); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Locale != "" {
		t.Fatalf("fresh workspace locale %q, want \"\" (English)", cfg.Locale)
	}

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	// First sync mirrors the seeded project in English and records the
	// locale the display names were fetched under.
	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync (en): %v\n%s", err, logs.String())
	}

	// An issue created while the workspace is still English: after the
	// locale switch its updated timestamp will be behind the watermark, so
	// only a full rebuild — not the incremental pass — can re-fetch it.
	enOut, err := capture(t, func() error { return cmdCreate([]string{"english era"}) })
	if err != nil {
		t.Fatalf("create (en): %v\n%s", err, enOut)
	}
	enKey := strings.Split(strings.TrimSpace(strings.Split(enOut, "\n")[0]), "\t")[0]
	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync #2 (en): %v\n%s", err, logs.String())
	}
	sqlOut, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header", "select key, status from issues_full where key = '" + enKey + "'"})
	})
	if err != nil {
		t.Fatalf("sql (en): %v\n%s", err, sqlOut)
	}
	if !strings.Contains(sqlOut, "To Do") {
		t.Fatalf("pre-switch mirror is not English:\n%s", sqlOut)
	}

	// Switch the workspace language.
	if _, err := capture(t, func() error { return cmdConfig([]string{"set", "locale", "ko"}) }); err != nil {
		t.Fatalf("config set locale ko: %v", err)
	}

	// status shows the locale next to the actor row.
	stOut, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v\n%s", err, stOut)
	}
	if !strings.Contains(stOut, "locale") || !strings.Contains(stOut, "ko") {
		t.Fatalf("status does not show the locale:\n%s", stOut)
	}

	// Create through the recorded type ID. Under ko the display name is
	// Korean, so name-keyed creation would be locale-fragile; the ID is not.
	koOut, err := capture(t, func() error {
		return cmdCreate([]string{"korean era", "--type", cfg.DefaultIssueTypeID})
	})
	if err != nil {
		t.Fatalf("create (ko, type id): %v\n%s", err, koOut)
	}
	koKey := strings.Split(strings.TrimSpace(strings.Split(koOut, "\n")[0]), "\t")[0]

	// The next sync must announce the rebuild — no silent refetch.
	logs.Reset()
	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync (ko): %v\n%s", err, logs.String())
	}
	if !strings.Contains(logs.String(), "rebuilding the mirror") {
		t.Fatalf("locale change did not announce a rebuild:\n%s", logs.String())
	}

	// The rebuild is what re-fetches the English-era row: query BOTH keys by
	// status_category — the stable id, never the display name — and expect
	// Korean statuses with English priority names (Cloud fidelity). Per row:
	// the English-era issue's `updated` is behind the watermark, so a pass
	// that skips unchanged rows would leave it in English — a mixed mirror.
	sqlOut, err = capture(t, func() error {
		return cmdSQL([]string{"--no-header",
			"select key, status, priority from issues_full where status_category = 'new' and key in ('" + enKey + "', '" + koKey + "')"})
	})
	if err != nil {
		t.Fatalf("sql (ko): %v\n%s", err, sqlOut)
	}
	rows := map[string][2]string{}
	for _, line := range strings.Split(strings.TrimSpace(sqlOut), "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) == 3 {
			rows[f[0]] = [2]string{f[1], f[2]}
		}
	}
	for _, key := range []string{enKey, koKey} {
		cols, ok := rows[key]
		if !ok {
			t.Fatalf("rebuild lost %s — mirror is mixed-language:\n%s", key, sqlOut)
		}
		if cols[0] != "해야 할 일" {
			t.Fatalf("%s status %q, want Korean — row not rewritten by the rebuild:\n%s", key, cols[0], sqlOut)
		}
		if cols[1] != "Medium" {
			t.Fatalf("%s priority %q, want English (Cloud fidelity):\n%s", key, cols[1], sqlOut)
		}
	}

	// A steady-state sync after the rebuild is incremental again: the
	// marker now says ko, so no second rebuild notice.
	logs.Reset()
	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync #4 (ko): %v\n%s", err, logs.String())
	}
	if strings.Contains(logs.String(), "rebuilding the mirror") {
		t.Fatalf("rebuild fires every sync — locale marker not recorded:\n%s", logs.String())
	}
}

// TestStandaloneLocalePersistedRoundtrip reopens the workspace from disk
// with the setting in place: the persist file's own locale must not win —
// gadak owns the workspace language — and a fresh process syncs in Korean
// without another rebuild.
func TestStandaloneLocalePersistedRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	}); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	if _, err := capture(t, func() error { return cmdConfig([]string{"set", "locale", "ko"}) }); err != nil {
		t.Fatalf("config set locale ko: %v", err)
	}
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })
	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync: %v\n%s", err, logs.String())
	}

	// Close the live session, reopen from persist: the store must still
	// speak Korean without a rebuild (marker already ko from the first sync).
	if err := origin.Close(); err != nil {
		t.Fatalf("origin.Close: %v", err)
	}
	logs.Reset()
	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync after reopen: %v\n%s", err, logs.String())
	}
	if strings.Contains(logs.String(), "rebuilding the mirror") {
		t.Fatalf("reopen triggered a spurious rebuild:\n%s", logs.String())
	}

	// And the status JSON carries the locale for machine readers.
	var st struct {
		Locale string `json:"locale"`
	}
	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("status json: %v\n%s", err, out)
	}
	if st.Locale != "ko" {
		t.Fatalf("status --json locale %q, want ko", st.Locale)
	}
}
