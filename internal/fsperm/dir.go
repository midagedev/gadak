// Package fsperm is the single owner of private-directory creation.
// Config and store both need 0700 data dirs; older 0755 installs are
// tightened, owner-locked (0555) dirs are not silently unlocked.
package fsperm

import (
	"errors"
	"fmt"
	"os"
)

// ErrChmod means the directory exists but chmod 0700 failed. Callers log
// the error and continue so unsupported filesystems (and Windows) still work.
var ErrChmod = errors.New("chmod")

// EnsurePrivateDir creates dir at 0700 and tightens an existing owner-writable
// directory to 0700 so older installs left at 0755 are quietly locked down
// (migration of H-2). It does not add owner-write: a 0555 (or otherwise
// owner-locked) directory stays locked.
//
// MkdirAll and Stat failures are returned as-is. A chmod failure is wrapped
// with ErrChmod so the caller can log it and continue.
func EnsurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o200 == 0 {
		return nil
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("%w %s: %w", ErrChmod, dir, err)
	}
	return nil
}
