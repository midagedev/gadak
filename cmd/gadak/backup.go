package main

// gadak backup — one consistent copy of the built-in tracker's persist file.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// cmdBackup copies the built-in tracker's persist (origin/issuetap.db) to
// one self-contained SQLite file. The mirror (gadak.db) is a cache and is
// not what this backs up; the persist is the record (GDK-1277).
//
// VACUUM INTO — the same primitive copyMirror uses — takes a read
// transaction, so it sees what is still in the -wal and needs no serve
// stop; the output has no sidecars. Copying via a running serve is the
// normal case, not an edge case.
func cmdBackup(args []string) error {
	const backupUsage = "usage: gadak backup [--to <dir|file>] [--json]"
	fs := newFlagSet("backup")
	to := fs.String("to", "", "destination directory (timestamped file inside) or file path; default: current directory")
	asJSON := fs.Bool("json", false, "print {path, issues, bytes} as JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("backup", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usageError("backup", backupUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.OriginType() != config.OriginGadak {
		return fmt.Errorf("backup is the built-in tracker's — this workspace's origin is %s, and that tracker holds the record; gadak.db here is a cache the next sync rebuilds", cfg.OriginType())
	}
	if cfg.Transport() == config.TransportRemote {
		label := "a serve on another machine"
		if rem, err := origin.PairedStatus(cfg); err == nil && rem != nil && rem.Label != "" {
			label = fmt.Sprintf("%q", rem.Label)
		}
		return fmt.Errorf("this workspace is paired with %s; the persist file lives there — run `gadak backup` on the home machine", label)
	}
	_, src := origin.Describe(cfg)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("nothing to back up yet: %w", err)
	}

	dst, err := backupTarget(*to)
	if err != nil {
		return err
	}
	if err := copyMirror(src, dst); err != nil {
		return fmt.Errorf("backup %s → %s: %w", src, dst, err)
	}

	st, err := os.Stat(dst)
	if err != nil {
		return err
	}
	n, err := backupIssueCount(dst)
	if err != nil {
		return fmt.Errorf("verify %s: %w", dst, err)
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(struct {
			Path   string `json:"path"`
			Issues int    `json:"issues"`
			Bytes  int64  `json:"bytes"`
		}{dst, n, st.Size()})
	}
	_, err = fmt.Fprintln(os.Stdout, dst)
	return err
}

// backupTarget resolves --to: an existing directory (or "") gets a
// timestamped file inside it; anything else is the file itself. Absolute,
// because the path is the command's whole stdout and scripts consume it.
func backupTarget(to string) (string, error) {
	if to == "" {
		to = "."
	}
	if st, err := os.Stat(to); err == nil && st.IsDir() {
		name := fmt.Sprintf("issuetap-%s-%s.db", workspaceJSONName(), time.Now().UTC().Format("20060102T150405Z"))
		to = filepath.Join(to, name)
	}
	return filepath.Abs(to)
}

func backupIssueCount(path string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int
	err = db.QueryRow("SELECT count(*) FROM issues").Scan(&n)
	return n, err
}
