// Package atomicfile is the one owner of stage-then-rename writes. The
// staging name must be unique per call: a fixed "<path>.tmp" let two
// concurrent savers of the same file truncate each other's staging copy,
// so half-written bytes could reach a credential document by rename
// (GDK-1233; GDK-1244 for the sites outside config). os.CreateTemp opens
// 0600 with O_EXCL — the mode every caller here wants — where os.WriteFile
// wrote through whatever already sat at the fixed name.
package atomicfile

import (
	"os"
	"path/filepath"
)

// WriteFile stages data through a unique temp file beside path (same
// directory, so the rename stays on one filesystem) and renames it into
// place. pattern names the temp file the way os.CreateTemp takes it, e.g.
// "config-*.json". On any failure before the rename the staging file is
// removed; after it, the remove is a no-op on a name that no longer exists.
func WriteFile(path, pattern string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
