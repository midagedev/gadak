package main

// The write ledger end-to-end (GDK-1440): real verbs on a built-in
// workspace, read back from local.agent_writes through the read-only
// surface export uses. The clauses:
//
//	L1 landed verbs leave rows — TestAgentWriteLedgerRecordsVerbs
//	     ① create → "create", claim → "claim", comment → "comment",
//	       close → "close"; key and source land with them
//	L2 dry runs leave no rows — TestAgentWriteLedgerSkipsDryRun
//	     ① comment --dry-run changes nothing
//	L3 export carries both sections — TestExportCarriesSessionsAndLedger
//	     ① gadak_export is 3, sessions and agent_writes arrays present
//
// FAIL-first evidence: against the pre-change tree these do not compile
// (agent_writes is not a table any query can read there — the store-level
// capture in internal/store covers the API; here the same archive run
// fails on the missing local.agent_writes table the moment the ledger
// query runs), failfirst/sessions-table-failfirst.out.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// ledgerRows reads (key, verb, source) from local.agent_writes, in row order.
func ledgerRows(t *testing.T) [][3]string {
	t.Helper()
	ro, err := openReadOnly()
	if err != nil {
		t.Fatalf("openReadOnly: %v", err)
	}
	defer ro.Close()
	rows, err := ro.Query(`SELECT key, verb, source FROM local.agent_writes ORDER BY id`)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	defer rows.Close()
	var out [][3]string
	for rows.Next() {
		var r [3]string
		if err := rows.Scan(&r[0], &r[1], &r[2]); err != nil {
			t.Fatalf("scan ledger: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return out
}

// ledgerEnv is the built-in workspace every ledger test shares: a fresh
// home, no ambient credentials, the embedded origin.
func ledgerEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --local: %v", err)
	}
}

func TestAgentWriteLedgerRecordsVerbs(t *testing.T) {
	ledgerEnv(t)
	created, err := capture(t, func() error { return cmdCreate([]string{"ledger roundtrip"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	if _, err := capture(t, func() error { return cmdClaim([]string{key}) }); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := capture(t, func() error { return cmdComment([]string{key, "-m", "ledger probe"}) }); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if _, err := capture(t, func() error { return cmdClose([]string{key}) }); err != nil {
		t.Fatalf("close: %v", err)
	}

	rows := ledgerRows(t)
	want := [][3]string{
		{key, "create", "cli"},
		{key, "claim", "cli"},
		{key, "comment", "cli"},
		{key, "close", "cli"},
	}
	if len(rows) != len(want) {
		t.Fatalf("ledger rows = %d, want %d:\n%v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Fatalf("ledger[%d] = %v, want %v", i, rows[i], w)
		}
	}
}

func TestAgentWriteLedgerSkipsDryRun(t *testing.T) {
	ledgerEnv(t)
	created, err := capture(t, func() error { return cmdCreate([]string{"dry run probe"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	if _, err := capture(t, func() error { return cmdComment([]string{key, "-m", "never lands", "--dry-run"}) }); err != nil {
		t.Fatalf("comment --dry-run: %v", err)
	}
	rows := ledgerRows(t)
	if len(rows) != 1 || rows[0][1] != "create" {
		t.Fatalf("ledger after a dry run = %v, want only the create row", rows)
	}
}

func TestExportCarriesSessionsAndLedger(t *testing.T) {
	ledgerEnv(t)
	created, err := capture(t, func() error { return cmdCreate([]string{"export v3 probe"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	var doc personalExport
	out, err := capture(t, func() error { return cmdExport([]string{}) })
	if err != nil {
		t.Fatalf("export: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("export json: %v\n%s", err, out)
	}
	if doc.Version != personalExportVersion {
		t.Fatalf("gadak_export = %d, want %d", doc.Version, personalExportVersion)
	}
	if len(doc.AgentWrites) == 0 {
		t.Fatal("export carries no agent_writes — the ledger section is missing")
	}
	if doc.AgentWrites[0].Key != key || doc.AgentWrites[0].Verb != "create" {
		t.Fatalf("agent_writes[0] = %+v, want key %s verb create", doc.AgentWrites[0], key)
	}
	// The create landed with no person session open, so the sessions section
	// is present but honestly empty — an array, not a null.
	if doc.Sessions == nil {
		t.Fatal("export sessions section is null, want an empty array")
	}
}
