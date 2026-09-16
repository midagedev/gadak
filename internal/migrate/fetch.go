package migrate

// Where the byte pass reads attachment bytes (GDK-1960).
//
// The pass used to believe bytes live at the origin and nowhere else, so a
// source whose origin could not be reached — frozen for the cutover, its
// credential revoked, the demo snapshot with no site at all — was refused
// even when every attachment the user had ever opened was sitting in that
// workspace's own cache on this disk. The refusal itself was right (GDK-1275:
// empty metadata masquerading as a migration); the premise was not. This
// file is the single owner of the corrected belief: origin first, the source
// workspace's cache second, the cache alone when the origin is gone.
//
// The cache key is formed here and nowhere else —
// attachcache.Key(site, profile, issue key, content id), the rule the
// server keys by (internal/server/attachment.go) and the snapshot import
// writes by (cmd/gadak demo.go). It is also why StreamFetch carries the
// issue key: the owner part of that key cannot be derived from a content id.

import (
	"context"
	"fmt"
	"io"

	"github.com/midagedev/gadak/internal/attachcache"
)

// AttachSource names the two places one attachment's bytes can come from.
// Origin is the source origin's stream; nil with Err saying why when the
// origin cannot be reached. Cache is the source workspace's own attachment
// cache; nil when there is none to consult. Site and Profile are the source
// workspace's identity — the same strings its server keyed its cache by, not
// the running workspace's.
type AttachSource struct {
	From    string             // source workspace name, for the refusal text
	Origin  StreamFetch        // nil when the origin is unreachable
	Err     error              // why Origin is nil, when it is
	Cache   *attachcache.Cache // nil when there is no cache to consult
	Site    string
	Profile string
}

// Fetch folds the source into the StreamFetch the write pass takes — or
// refuses the run before a byte moves, the way the cmd used to (GDK-1275:
// the refusal has to come before the export, not after). The refusal only
// fires when the origin is unreachable and the cache cannot cover what the
// pass would otherwise fetch; a covered run proceeds on the cache alone,
// and a reachable origin never refuses — its misses fall back to the cache
// and only then count as misses (a file deleted at the origin but viewed
// once still travels).
//
// st is the same Stats the write pass reports from; the serve counts land
// in it as bytes are handed over.
func (s AttachSource) Fetch(doc *Doc, st *Stats) (StreamFetch, error) {
	if s.Origin == nil && s.Err != nil {
		fetchable, cached := s.coverage(doc)
		if cached < fetchable {
			return nil, fmt.Errorf("cannot read attachment bytes from %q: %w\n"+
				"  the source workspace's attachment cache can supply %d of %d attachments; %d would migrate as empty metadata, and the count table would not say so\n"+
				"  to bring the rest: make the source reachable (a frozen workspace: `gadak --workspace %s config set frozen false`)\n"+
				"  to migrate without them on purpose: --skip-attachments",
				s.From, s.Err, cached, fetchable, fetchable-cached, s.From)
		}
	}
	return func(ctx context.Context, issueKey, contentID string) (int, int64, io.ReadCloser, error) {
		if s.Origin != nil {
			status, size, body, err := s.Origin(ctx, issueKey, contentID)
			if err == nil && status == 200 && body != nil {
				st.AttachFromOrigin++
				return status, size, body, nil
			}
			// Anything else — a 404, a transport error — falls through to
			// the cache, and on a cache miss the origin's own answer goes
			// back out, so the counting (missing vs error) stays what it
			// was and never blames the cache for the origin's silence.
			if rc, meta, ok := s.cacheGet(issueKey, contentID); ok {
				st.AttachFromCache++
				return 200, meta.Size, rc, nil
			}
			return status, size, body, err
		}
		if rc, meta, ok := s.cacheGet(issueKey, contentID); ok {
			st.AttachFromCache++
			return 200, meta.Size, rc, nil
		}
		// No origin and no bytes: report the miss in the slot the count
		// line already explains, rather than inventing an error.
		return 404, 0, nil, nil
	}, nil
}

// cacheGet is the cache half: one lookup under one key, formed the way the
// server formed it when it wrote the bytes.
func (s AttachSource) cacheGet(issueKey, id string) (io.ReadSeekCloser, attachcache.Meta, bool) {
	if s.Cache == nil || issueKey == "" || id == "" {
		return nil, attachcache.Meta{}, false
	}
	rc, meta, err := s.Cache.Get(attachcache.Key(s.Site, s.Profile, issueKey, id))
	if err != nil {
		return nil, attachcache.Meta{}, false
	}
	return rc, meta, true
}

// coverage counts what the byte pass would try to read from doc — a
// Jira-sourced attachment under the size cap, the two gates writeAttachment
// applies before it fetches — and how many of those the cache already
// holds. It is the refusal's evidence, so it reads the doc rather than the
// stats: the question is "what could still go missing", not "what was
// counted".
func (s AttachSource) coverage(doc *Doc) (fetchable, cached int) {
	for i := range doc.Issues {
		for _, a := range doc.Issues[i].Attachments {
			if a.SourceURL != "" || a.Size > maxAttachmentBytes {
				continue
			}
			fetchable++
			if s.Cache != nil && s.Cache.Has(attachcache.Key(s.Site, s.Profile, doc.Issues[i].Key, a.ContentID)) {
				cached++
			}
		}
	}
	return fetchable, cached
}
