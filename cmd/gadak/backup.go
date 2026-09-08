package main

// gadak backup — one consistent copy of the built-in tracker's record.
//
// That record used to be one SQLite file, and this command copied it.
// Attachment bytes now live in a directory beside it (GDK-1617), so a copy
// of the file alone is a backup with every attachment missing and nothing
// saying so — worse than no backup, because it looks like one. The output
// is a tar holding both.

import (
	"archive/tar"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// cmdBackup writes one self-contained tar of the built-in tracker: the
// persist database and the attachment bytes directory. The mirror
// (gadak.db) is a cache and is not what this backs up; the persist plus
// its blobs are the record (GDK-1277, GDK-1617).
//
// VACUUM INTO — the same primitive copyMirror uses — takes a read
// transaction, so it sees what is still in the -wal and needs no serve
// stop; the output has no sidecars. Copying via a running serve is the
// normal case, not an edge case.
//
// The database goes into the archive BEFORE the blobs. Writes land as
// file-then-row, so every row in the snapshot already has its file: taking
// the database first cannot capture a reference to a blob the archive then
// misses. The other order can.
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
	n, err := writeBackupArchive(src, filepath.Join(filepath.Dir(src), "blobs"), dst)
	if err != nil {
		return fmt.Errorf("backup %s → %s: %w", src, dst, err)
	}

	st, err := os.Stat(dst)
	if err != nil {
		return err
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
		name := fmt.Sprintf("issuetap-%s-%s.tar", workspaceJSONName(), time.Now().UTC().Format("20060102T150405Z"))
		to = filepath.Join(to, name)
	}
	return filepath.Abs(to)
}

// writeBackupArchive builds the tar and returns the issue count it saw.
// Verification is not "the tar exists": it is that every sha the database
// references is a member of the archive. A backup nobody checked is the
// same object as no backup.
func writeBackupArchive(persist, blobDir, dst string) (n int, err error) {
	// Anything that fails after the file exists takes it with it. The
	// verify below runs after the members are written, so a refusal used
	// to leave a truncated .tar at the destination — and O_EXCL then
	// turned "an existing target is never overwritten" into a guard over a
	// known-bad artifact, blocking the retry (GDK-1617 review).
	created := false
	defer func() {
		// Only what this call created. Removing dst unconditionally would
		// delete the user's existing backup on the very path that exists
		// to protect it — the O_EXCL refusal below.
		if err != nil && created {
			_ = os.Remove(dst)
		}
	}()
	tmpDB := dst + ".db.tmp"
	defer os.Remove(tmpDB)
	if err := copyMirror(persist, tmpDB); err != nil {
		return 0, err
	}
	n, err = backupIssueCount(tmpDB)
	if err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}
	want, err := backupReferencedSHAs(tmpDB)
	if err != nil {
		return 0, fmt.Errorf("verify: %w", err)
	}

	// O_EXCL: an existing target is refused, never overwritten. Someone
	// naming a path that is already a backup means to make a new one, not
	// to destroy the one they have.
	out, oerr := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if oerr != nil {
		return 0, oerr
	}
	created = true
	defer out.Close()
	tw := tar.NewWriter(out)

	if err := tarAddFile(tw, "issuetap.db", tmpDB); err != nil {
		return 0, err
	}
	have := map[string]bool{}
	err = filepath.Walk(blobDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == blobDir {
				return nil // no attachments yet
			}
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(blobDir, p)
		if rerr != nil {
			return rerr
		}
		if strings.HasPrefix(rel, "tmp"+string(os.PathSeparator)) {
			return nil // debris from a dead upload, not content
		}
		have[fi.Name()] = true
		return tarAddFile(tw, path.Join("blobs", filepath.ToSlash(rel)), p)
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for sha := range want {
		if !have[sha] {
			return 0, fmt.Errorf("verify: the database references attachment %s but its bytes are not on disk — the archive would be missing them", sha)
		}
	}
	if err := tw.Close(); err != nil {
		return 0, err
	}
	return n, out.Close()
}

func tarAddFile(tw *tar.Writer, name, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o600, Size: fi.Size(), ModTime: fi.ModTime(), Typeflag: tar.TypeReg,
	}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// backupReferencedSHAs is empty on a persist this build has not migrated —
// an older file still holds its bytes inside the database, and copying it
// is a complete backup on its own.
func backupReferencedSHAs(path string) (map[string]bool, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT DISTINCT sha256 FROM attachment_blobs`)
	if err != nil {
		return map[string]bool{}, nil //nolint:nilerr // pre-v2 persist has no such table
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
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
