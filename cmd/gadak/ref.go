package main

// `gadak ref` — cross-workspace issue references (GDK-1032). A local-origin
// or paired issue points at an issue in another workspace, the pointer is
// stored on this origin as a Jira remote issue link, and the target's
// current state is hydrated from that workspace's own mirror with no
// network call. Nothing is ever written to the target workspace: a personal
// note about a team issue stays personal.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

const refUsage = "usage: gadak ref <KEY> <workspace>/<TARGET-KEY>|<url> [--as <relationship>] [--json]\n" +
	"       gadak ref <KEY> --list [--json]\n" +
	"       gadak ref <KEY> --rm <id>"

// refScheme is the URL form a cross-workspace pointer takes:
// gadak://<workspace>/<KEY>. It is the stored identity, so the hydrator can
// recognize its own rows among ordinary remote links (a plain https:// URL
// is a valid pointer too — it just has nothing local to hydrate from).
const refScheme = "gadak://"

func cmdRef(args []string) error {
	fs := newFlagSet("ref")
	as := fs.String("as", "", "relationship phrase stored with the pointer (default: relates to)")
	list := fs.Bool("list", false, "list this issue's references")
	rm := fs.String("rm", "", "remove the reference with this id (from --list)")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("ref", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError("ref", refUsage)
	}
	key := normalizeKey(pos[0])
	rest := pos[1:]

	switch {
	case *list:
		if len(rest) != 0 {
			return usageError("ref", refUsage)
		}
		return refList(key, *asJSON)
	case *rm != "":
		if len(rest) != 0 {
			return usageError("ref", refUsage)
		}
		return refRemove(key, *rm, *asJSON)
	case len(rest) == 1:
		return refAdd(key, rest[0], *as, *asJSON)
	default:
		return usageError("ref", refUsage)
	}
}

// parseRefTarget turns `work/NMA-9`, `NMA-9` (another workspace implied by
// nothing — refused), or a plain URL into the stored pointer.
func parseRefTarget(token string) (url, workspace, targetKey string, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", "", fmt.Errorf("name what to point at: <workspace>/<KEY> or a URL")
	}
	if strings.Contains(token, "://") {
		if strings.HasPrefix(token, refScheme) {
			ws, k, ok := strings.Cut(strings.TrimPrefix(token, refScheme), "/")
			if !ok || ws == "" || k == "" {
				return "", "", "", fmt.Errorf("%s is not <workspace>/<KEY>", token)
			}
			return token, ws, normalizeKey(k), nil
		}
		return token, "", "", nil
	}
	ws, k, ok := strings.Cut(token, "/")
	if !ok || ws == "" || k == "" {
		return "", "", "", fmt.Errorf("point at <workspace>/<KEY> (e.g. work/NMA-9) or a URL — %q is neither", token)
	}
	k = normalizeKey(k)
	return refScheme + ws + "/" + k, ws, k, nil
}

func refAdd(key, target, relationship string, asJSON bool) error {
	url, ws, targetKey, err := parseRefTarget(target)
	if err != nil {
		return err
	}
	if relationship == "" {
		relationship = "relates to"
	}
	title := url
	summary := ""
	if targetKey != "" {
		title = targetKey
		// The target's summary is read once at write time so the pointer
		// reads sensibly even from a machine that does not mirror that
		// workspace. The live state still comes from hydration.
		if lite, ok := hydrateRef(ws, targetKey); ok {
			summary = lite.summary
			title = targetKey + " — " + lite.summary
		}
	}

	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		rl, err := origin.AsRemoteLinker(cfg, c)
		if err != nil {
			return err
		}
		globalID := ""
		if targetKey != "" {
			globalID = url
		}
		if err := rl.SetRemoteLink(ctx, key, origin.RemoteLink{
			GlobalID: globalID, Relationship: relationship,
			URL: url, Title: title, Summary: summary,
		}); err != nil {
			return refOriginTooOld(cfg, err)
		}
		if err := refreshRefs(ctx, cfg, db, rl, key); err != nil {
			return err
		}
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON,
			map[string]any{"ref": map[string]any{"url": url, "relationship": relationship, "title": title}})
	})
}

func refRemove(key, id string, asJSON bool) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		rl, err := origin.AsRemoteLinker(cfg, c)
		if err != nil {
			return err
		}
		if err := rl.DeleteRemoteLink(ctx, key, id); err != nil {
			return refOriginTooOld(cfg, err)
		}
		if err := refreshRefs(ctx, cfg, db, rl, key); err != nil {
			return err
		}
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON, map[string]any{"removed": id})
	})
}

// refOriginTooOld turns the origin's raw 501 into the sentence that names
// the fix. A paired workspace's origin is another machine's gadak, and a
// home serve older than this binary answers "does not implement
// …/remotelink" — true, and useless: what the person needs to know is
// which machine to upgrade (measured on a paired workspace the day the
// verb landed).
func refOriginTooOld(cfg *config.Config, err error) error {
	if err == nil || !strings.Contains(err.Error(), "remotelink") ||
		!strings.Contains(err.Error(), "501") {
		return err
	}
	where := "this workspace's origin"
	if rem, perr := origin.PairedStatus(cfg); perr == nil && rem != nil {
		where = fmt.Sprintf("the gadak serve this workspace is paired with (%s)", rem.Label)
	}
	return fmt.Errorf("references need a newer origin: %s does not serve remote issue links yet — upgrade gadak there (this machine is %s)", where, version)
}

// refreshRefs re-reads the origin's remote links for one issue and rewrites
// the mirror rows — the write-through half, same shape as a dev-link write.
func refreshRefs(ctx context.Context, cfg *config.Config, db *store.DB, rl origin.RemoteLinker, key string) error {
	links, err := rl.RemoteLinks(ctx, key)
	if err != nil {
		return err
	}
	update := store.RemoteLinksUpdate{}
	for _, l := range links {
		update.Links = append(update.Links, store.RemoteLink{
			ID: l.ID, GlobalID: l.GlobalID, Relationship: l.Relationship,
			URL: l.URL, Title: l.Title, Summary: l.Summary,
		})
	}
	return db.ReplaceRemoteLinks(ctx, key, update)
}

// refRow is one listed reference, hydrated where the target is local.
type refRow struct {
	ID           string `json:"id"`
	Relationship string `json:"relationship,omitempty"`
	URL          string `json:"url"`
	Title        string `json:"title,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	Key          string `json:"key,omitempty"`
	// Live is the target's current state, read from that workspace's own
	// mirror at print time — the reason this feature exists here and not
	// in a browser tab.
	Status   string `json:"status,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Stale    bool   `json:"stale,omitempty"`
}

func refList(key string, asJSON bool) error {
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.RemoteLinks(context.Background(), key)
	if err != nil {
		return err
	}
	out := make([]refRow, 0, len(rows))
	for _, r := range rows {
		row := refRow{ID: r.ID, Relationship: r.Relationship, URL: r.URL, Title: r.Title}
		if strings.HasPrefix(r.URL, refScheme) {
			ws, k, ok := strings.Cut(strings.TrimPrefix(r.URL, refScheme), "/")
			if ok {
				row.Workspace, row.Key = ws, k
				if lite, found := hydrateRef(ws, k); found {
					row.Status, row.Assignee, row.Summary = lite.status, lite.assignee, lite.summary
				} else {
					row.Stale = true
				}
			}
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"key": key, "refs": out})
	}
	if len(out) == 0 {
		fmt.Printf("%s has no references\n", key)
		return nil
	}
	for _, r := range out {
		target := r.Title
		if r.Key != "" {
			target = r.Workspace + "/" + r.Key
		}
		line := fmt.Sprintf("%s\t%s\t%s", r.ID, r.Relationship, target)
		switch {
		case r.Status != "":
			line += fmt.Sprintf("\t%s", r.Status)
			if r.Assignee != "" {
				line += "\t" + r.Assignee
			}
			if r.Summary != "" {
				line += "\t" + r.Summary
			}
		case r.Stale:
			// Not an error: the pointer is fine, this machine just does
			// not mirror that workspace (or has not synced it).
			line += "\t(not mirrored here)"
		}
		fmt.Println(line)
	}
	return nil
}

// refLite is what hydration reads out of another workspace's mirror.
type refLite struct {
	summary  string
	status   string
	assignee string
}

// hydrateRef reads one issue out of another workspace's mirror, read-only.
// A workspace that does not exist, has no mirror, or does not carry the key
// is a miss, never an error — the pointer stays valid either way.
func hydrateRef(workspace, key string) (refLite, bool) {
	if workspace == "" || key == "" {
		return refLite{}, false
	}
	path, err := config.DBPathFor(workspace)
	if err != nil {
		return refLite{}, false
	}
	if _, err := os.Stat(path); err != nil {
		return refLite{}, false
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		return refLite{}, false
	}
	defer db.Close()
	var lite refLite
	err = db.QueryRow(`
		SELECT COALESCE(summary,''), COALESCE(status,''), COALESCE(assignee,'')
		FROM issues_full WHERE key = ? LIMIT 1`, key).Scan(&lite.summary, &lite.status, &lite.assignee)
	if err != nil {
		return refLite{}, false
	}
	return lite, true
}
