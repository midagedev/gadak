package main

// GDK-1610: reading an attachment had no verb. `gadak issue KEY` already
// lists them — filename, type, size — but getting the bytes meant parsing
// `issues.raw` for a numeric id and hand-assembling a REST path for
// `gadak api`. Three steps, every time someone opens a QA reproduction.
//
// This is the read half of `attach`. It does not invent a lister: the
// listing exists.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

const attachGetUsage = "usage: gadak attach get <KEY> <filename|id> [--out DIR|FILE|-] [--json]"

// cmdAttachGet writes one mirrored attachment's bytes to a file (or stdout).
func cmdAttachGet(args []string) error {
	fs := newFlagSet("attach get")
	out := fs.String("out", "", "destination: a directory, a file path, or - for stdout (default: the filename, here)")
	asJSON := fs.Bool("json", false, "emit JSON")
	force := fs.Bool("force", false, "overwrite an existing file")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("attach", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return usageError("attach get", attachGetUsage)
	}
	key := normalizeKey(pos[0])
	want := pos[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	att, err := resolveAttachment(ctx, db, key, want)
	if err != nil {
		return err
	}

	body, err := openAttachment(ctx, cfg, db, key, att)
	if err != nil {
		return err
	}
	defer body.Close()

	dest, w, err := attachDest(*out, att.Filename, *force)
	if err != nil {
		return err
	}
	n, err := io.Copy(w, body)
	if cerr := closeDest(w); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if dest == "" {
		return nil // stdout: the bytes are the output
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"key": key, "id": att.ExternalID, "filename": att.Filename, "path": dest, "bytes": n})
	}
	fmt.Printf("%s\t%d bytes\n", dest, n)
	return nil
}

// resolveAttachment finds one of the issue's mirrored attachments by
// filename or by id (either the mirror's namespaced id or the origin's own
// external id). Membership is the query: an id that belongs to a different
// issue is simply not found, so a key/id mismatch cannot read bytes.
func resolveAttachment(ctx context.Context, db *store.DB, key, want string) (store.DetailAttachment, error) {
	d, err := db.Detail(ctx, key)
	if err != nil {
		return store.DetailAttachment{}, err
	}
	var hits []store.DetailAttachment
	for _, a := range d.Attachments {
		if a.Filename == want || a.ExternalID == want || a.ID == want {
			hits = append(hits, a)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		if len(d.Attachments) == 0 {
			return store.DetailAttachment{}, fmt.Errorf("%s has no attachments in this mirror\n"+
				"  if the issue is newer than the last sync: gadak sync", key)
		}
		names := make([]string, 0, len(d.Attachments))
		for _, a := range d.Attachments {
			names = append(names, a.Filename)
		}
		return store.DetailAttachment{}, fmt.Errorf("%s has no attachment %q — it has: %s",
			key, want, strings.Join(names, ", "))
	default:
		ids := make([]string, 0, len(hits))
		for _, a := range hits {
			ids = append(ids, a.ExternalID)
		}
		return store.DetailAttachment{}, fmt.Errorf("%s has %d attachments named %q — name one by id: %s",
			key, len(hits), want, strings.Join(ids, ", "))
	}
}

// openAttachment streams the bytes from the origin this attachment came
// from. The branch is on the mirrored row's source, never a fallback: a
// Linear attachment is not reachable over the Jira REST path and asking
// there would answer 404 rather than say so.
func openAttachment(ctx context.Context, cfg *config.Config, db *store.DB, key string, att store.DetailAttachment) (io.ReadCloser, error) {
	sourceID, contentURL, err := db.AttachmentOrigin(ctx, key, att.ExternalID)
	if err != nil {
		return nil, err
	}
	if sourceID == "linear" {
		var apiKey string
		if cfg != nil && cfg.Linear != nil {
			apiKey = cfg.Linear.APIKey
		}
		if apiKey == "" {
			return nil, fmt.Errorf("this attachment is on Linear and no linear apiKey is configured: gadak config set linear.apiKey ...")
		}
		body, _, derr := linear.Download(ctx, contentURL, apiKey)
		return body, derr
	}
	// Jira-shaped origins — an Atlassian site, the built-in tracker in this
	// process, a self-hosted issuetap, a paired home serve — all answer the
	// same content path through one client (origin.Client).
	c, err := origin.Client(cfg)
	if err != nil {
		return nil, err
	}
	status, buf, err := c.Raw(ctx, http.MethodGet,
		"/rest/api/3/attachment/content/"+url.PathEscape(att.ExternalID), nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("attachment %s: HTTP %d: %s", att.ExternalID, status, httpStatusDetail(status, buf))
	}
	return io.NopCloser(strings.NewReader(string(buf))), nil
}

// attachDest resolves --out into a destination. "" is the filename in the
// working directory; a directory keeps the filename inside it; "-" is
// stdout. An existing file is refused unless --force: a download that
// silently replaces a file is the kind of thing you notice too late.
func attachDest(out, filename string, force bool) (string, io.WriteCloser, error) {
	if out == "-" {
		return "", nopWriteCloser{os.Stdout}, nil
	}
	dest := filename
	if out != "" {
		if fi, err := os.Stat(out); err == nil && fi.IsDir() {
			dest = filepath.Join(out, filename)
		} else {
			dest = out
		}
	}
	if _, err := os.Stat(dest); err == nil && !force {
		return "", nil, fmt.Errorf("%s exists — pass --force to overwrite, or --out to write elsewhere", dest)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", nil, err
	}
	return dest, f, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func closeDest(w io.WriteCloser) error { return w.Close() }
