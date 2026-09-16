package main

// `gadak demo --writable` (GDK-1959).
//
// The read-only demo is a workspace with no credential, which is exactly
// what makes it read-only: every write verb answers 409 credential_required
// and the web and phone clients paint their write controls disabled, because
// `GET credential/` tells them the serve cannot write. So a person evaluating
// gadak sees the whole product except the half it is for — no comment, no
// transition, no photo — and the store-review guideline that asks a demo to
// show the app's features (GDK-958) fails on the same sentence.
//
// The fix needs no new demo machinery: `migrate` already turns a mirror into
// a workspace on the built-in tracker, and a built-in origin is writable by
// construction (config.HasBuiltInOrigin → HasCredential, so the probe the
// clients read answers true). This file is the wiring, and deliberately
// nothing else — the temp home, the attachment import, the clock and the
// identity all stay owned by cmdDemo, and the export stays owned by
// cmdMigrate. A second copy of any of those is how the demo home ends up
// with two owners that drift.
//
// What the flag costs: the snapshot is copied once more (into the built-in
// origin's persist file) and the issues are re-mirrored from it, measured at
// about two seconds for the 534-issue fixture. What it buys: the demo takes
// writes, which is also what a recording needs to film a write and what a
// review server needs to demonstrate one.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/midagedev/gadak/internal/config"
)

// writableDemoWorkspace is the workspace name the writable demo serves. The
// read-only demo serves the default (unnamed) one; this is a second
// workspace inside the same throwaway home, so `gadak demo` and
// `gadak demo --writable` differ by which one is served and by nothing else.
const writableDemoWorkspace = "demo"

// makeDemoWritable turns the snapshot already sitting in the demo's throwaway
// home into a workspace on the built-in tracker, and leaves the process
// pointed at it so the caller's cmdServe serves that one. home is the temp
// directory cmdDemo minted; GADAK_HOME already names it.
//
// The migration runs the product's own verb, flags and all, so the demo can
// never carry bytes the documented command would not: attachment bytes come
// from the cache cmdDemo just imported (GDK-1960 — the origin here is
// unreachable by construction, so this is that path's first real user), and
// sprints travel through the Agile pass (GDK-1961), which is why the demo
// still opens on a sprint line.
// haveBytes says whether the snapshot's attachment bytes reached the demo
// home's cache. It is not a detail: the export refuses rather than migrate
// attachments as empty metadata (GDK-1275), and with no origin to ask, the
// cache is the only supplier. cmdDemo already knows the answer, so it says
// so here instead of letting the refusal surface as an unexplained failure
// three layers down.
// accountID is the identity the source snapshot resolved by email. It is
// passed rather than re-derived because the export carries account ids
// through but not emails: the demo fixture's users table is empty, so every
// account arrives as a ghost row whose display name is its id and whose
// email is blank (the migration report names them). Measured on the demo
// fixture: `demo-dana` is the assignee id on both sides, and
// `assignee_email` is empty on the target — so an email lookup that works
// on the snapshot answers nothing after the move.
func makeDemoWritable(home string, haveBytes bool, accountID string) error {
	// The export's counts are the provenance of what the demo now serves;
	// printing them is how a run says what moved rather than asserting it.
	fmt.Fprintln(os.Stderr, "demo: building a writable origin from the snapshot…")
	allowProfileCreate = true
	config.SetProfile(writableDemoWorkspace)
	args := []string{"--from", "default"}
	if !haveBytes {
		// A demo whose pictures did not arrive is worth having — it still
		// takes comments and transitions — but it is not worth having
		// quietly, which is the whole lesson of the refusal being skipped.
		fmt.Fprintln(os.Stderr, "demo: the snapshot's attachment bytes are not in this home's cache — the writable demo will show attachments as ledger rows, without images")
		args = append(args, "--skip-attachments")
	}
	if err := cmdMigrate(args); err != nil {
		return fmt.Errorf("build the writable demo origin: %w", err)
	}

	// The identity and the clock are cmdDemo's two corrections to a raw
	// snapshot, and the new workspace is a different config file, so both
	// have to land on it too. Identity first (GDK-1729: without an account
	// id nothing in the fixture is "mine" and the retro resume cell is a
	// dash); the id is read back out of the migrated mirror rather than
	// carried over, because the built-in tracker mints its own and a copied
	// one would match nothing.
	dbPath, err := config.DBPathFor(writableDemoWorkspace)
	if err != nil {
		return err
	}
	cfg, err := config.LoadFor(writableDemoWorkspace)
	if err != nil {
		return err
	}
	cfg.Email = demoUserEmail
	// A copied id that matches nothing is worse than no id: the resume row
	// would be a dash either way, but the config would claim otherwise. So
	// the id is only kept when the migrated mirror actually carries it.
	if accountID != "" {
		known, err := demoAccountIDPresent(dbPath, accountID)
		if err != nil {
			return err
		}
		if known {
			cfg.AccountID = accountID
		} else {
			fmt.Fprintf(os.Stderr, "demo: the snapshot's account id %q is not in the migrated mirror — the writable demo has no \"mine\"\n", accountID)
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	// Then the clock, for the same reason the read-only demo freshens its
	// own: a demo that opens on "Sync delayed" reads as a defect rather than
	// as the freshness guard it is. The migrated mirror was written just
	// now, but its rows carry the snapshot's timestamps.
	if err := freshenDemoClock(dbPath); err != nil {
		return err
	}
	// The mirror's own identity row is resolved when a command opens the
	// store, so nothing else is written here; what remains is proving the
	// origin the clients will probe really is writable, before a person
	// finds out by tapping a disabled control.
	if !cfg.HasCredential() {
		return fmt.Errorf("the writable demo workspace %q reports no credential — its clients would still paint every write control disabled", writableDemoWorkspace)
	}
	fmt.Fprintf(os.Stderr, "demo: serving the writable workspace %q — comments, transitions and attachments reach its own origin\n", writableDemoWorkspace)
	return nil
}

// demoAccountIDPresent reports whether the migrated mirror attributes any
// row to this account. It is the check that separates "the id travelled" from
// "the id was copied": the built-in tracker keeps the ids the export handed
// it, but a scrubbed or regenerated fixture need not, and a config naming an
// account nothing references reads as identity while providing none.
func demoAccountIDPresent(dbPath, accountID string) (bool, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.Clean(dbPath)+"?mode=ro")
	if err != nil {
		return false, err
	}
	defer db.Close()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM issues_full WHERE assignee_id = ? OR reporter_id = ?`,
		accountID, accountID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// demoIssueCount reports how many issues the workspace's mirror holds. Used
// by the writable path's own check that the migration actually filled the
// mirror it is about to serve: a workspace that exists but mirrored nothing
// serves an empty list, which looks like a broken product rather than a
// failed step.
func demoIssueCount(dbPath string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.Clean(dbPath)+"?mode=ro")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM issues_full`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
