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
	"github.com/midagedev/gadak/internal/fields"
	"io"
	"mime"
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
	key := fields.CanonicalKey(pos[0])
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
	// Stream, not Raw. Raw reads through a 64 MiB io.LimitReader and
	// returns the prefix with no error — measured: a 200 MiB attachment
	// downloaded as exactly 67,108,864 bytes, a different hash, and exit 0
	// (GDK-1617). The bytes on the origin were intact; the CLI truncated
	// them on the way out, which is the same silent-loss shape GDK-1614
	// fixed on the way in. Streaming also means the file never has to fit
	// in memory: the same download peaked at 526 MB RSS before this.
	// Jira Server has no /attachment/content route: the bytes live at the
	// URL the origin stated, which sync recorded (GDK-1639). Reducing it to
	// a site-relative path is what keeps the Authorization header on this
	// workspace's site.
	path := "/rest/api/3/attachment/content/" + url.PathEscape(att.ExternalID)
	if cfg.OriginType() == config.OriginJiraServer {
		path, err = origin.SiteRelative(c.BaseURL(), contentURL)
		if err != nil {
			return nil, err
		}
	}
	res, err := c.Stream(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		buf, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		res.Body.Close()
		return nil, fmt.Errorf("attachment %s: HTTP %d: %s", att.ExternalID, res.StatusCode, httpStatusDetail(res.StatusCode, buf))
	}
	if err := refuseErrorPage(att, res.Header.Get("Content-Type")); err != nil {
		res.Body.Close()
		return nil, err
	}
	return res.Body, nil
}

// refuseErrorPage rejects a 200 that is an HTML page where the mirror says
// the attachment is something else (GDK-1644).
//
// A status check is not enough: an origin that does not serve this route can
// answer the request with its own login or dashboard page, at 200, and the
// bytes then get written to the destination file under the attachment's
// name. Measured against Jira Server 11.3.11, which has no
// /attachment/content route: `gadak attach get … --out x.png` wrote 257,592
// bytes of HTML as x.png and exited 0.
//
// The comparison is against the mirror's own mime type rather than a blanket
// "HTML is not a file": .html is a perfectly ordinary thing to attach, and
// refusing those would trade a silent wrong file for a refused right one.
func refuseErrorPage(att store.DetailAttachment, contentType string) error {
	served, _, err := mime.ParseMediaType(contentType)
	if err != nil || served != "text/html" {
		return nil
	}
	recorded, _, err := mime.ParseMediaType(att.MimeType)
	if err == nil && recorded == "text/html" {
		return nil
	}
	return fmt.Errorf("attachment %s (%s): the origin answered with an HTML page, not the file — its type is %q here. Nothing was written",
		att.ExternalID, att.Filename, att.MimeType)
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
