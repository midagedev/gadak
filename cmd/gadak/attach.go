package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

const attachUsage = "usage: gadak attach <KEY> <file>... [--json]"

// attachedFile is the JSON/extra row emitAfterWrite carries as "attached".
type attachedFile struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
}

// attachPartialError is a mid-list upload failure. The writes that landed
// already happened; the caller must not retry them.
type attachPartialError struct {
	landed    []string
	failed    string
	remaining []string
	err       error
}

func (e *attachPartialError) Error() string {
	not := append([]string{e.failed}, e.remaining...)
	if len(e.landed) == 0 {
		return fmt.Sprintf("attaching %s failed: %v", e.failed, e.err)
	}
	return fmt.Sprintf("attaching %s failed: %v (landed: %s; not attached: %s)",
		e.failed, e.err, strings.Join(e.landed, ", "), strings.Join(not, ", "))
}

func (e *attachPartialError) Unwrap() error { return e.err }

func cmdAttach(args []string) error {
	fs := newFlagSet("attach")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("attach", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageError("attach", attachUsage)
	}
	key := normalizeKey(pos[0])
	paths := pos[1:]
	if err := validateAttachPaths(paths); err != nil {
		return err
	}
	return withWriteSession(func(ctx context.Context, cfg *config.Config, db *store.DB, c *jira.Client) error {
		attached, err := uploadAttachPaths(ctx, c, key, paths)
		if err != nil {
			return err
		}
		if err := emitAfterWrite(ctx, cfg, db, c, key, *asJSON, map[string]any{"attached": attached}); err != nil {
			return err
		}
		if !*asJSON {
			for _, a := range attached {
				fmt.Printf("  + %s\n", a.Filename)
			}
		}
		return nil
	})
}

// validateAttachPaths checks every path exists and is a regular file (Stat
// follows a symlink) before any upload starts.
func validateAttachPaths(paths []string) error {
	var errs []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		switch {
		case err != nil:
			errs = append(errs, err.Error())
		case !fi.Mode().IsRegular():
			errs = append(errs, fmt.Sprintf("%s: not a regular file", p))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("cannot attach: %s", strings.Join(errs, "; "))
}

// uploadAttachPaths sends each file through Client.Upload in order. Upload
// itself buffers the bytes (see the ponytail on jira.Client.Upload); this
// helper opens the path and does not ReadAll first.
func uploadAttachPaths(ctx context.Context, c *jira.Client, key string, paths []string) ([]attachedFile, error) {
	attached := make([]attachedFile, 0, len(paths))
	landed := make([]string, 0, len(paths))
	for i, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return attached, &attachPartialError{
				landed: landed, failed: path, remaining: append([]string{}, paths[i+1:]...), err: err,
			}
		}
		atts, err := c.Upload(ctx, key, filepath.Base(path), file)
		_ = file.Close()
		if err != nil {
			return attached, &attachPartialError{
				landed: landed, failed: path, remaining: append([]string{}, paths[i+1:]...), err: err,
			}
		}
		for _, a := range atts {
			name := a.Filename
			if name == "" {
				name = filepath.Base(path)
			}
			attached = append(attached, attachedFile{ID: a.ID, Filename: name})
			landed = append(landed, name)
		}
	}
	return attached, nil
}
