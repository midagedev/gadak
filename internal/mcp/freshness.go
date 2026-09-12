package mcp

import (
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/freshness"
)

// GDK-599: the fact "this mirror is behind" used to reach only stderr, and
// an MCP host cannot see stderr — the initialize instruction "call
// gadak_status before acting" is advice, not a guarantee, so an agent that
// skipped it answered from a stale mirror with full confidence. The notice
// rides the result instead: appended, never merged, so the first content
// item stays byte-identical and a JSON-parsing client sees no change.

// carriesFreshnessNotice reports whether tools/list's tool by this name
// answers from the mirror and so carries the freshness notice. The whole set,
// and the whole exclusion list, in one place:
//
//   - gadak_query, gadak_search, gadak_issue, gadak_retro answer from the
//     mirror; gadak_show and gadak_recents answer from this workspace's
//     state. A stale mirror indicts every one of them.
//   - gadak_status is excluded because its payload IS the freshness answer —
//     a notice would say what the result already says in more detail.
//   - the ui.tokens pair is excluded because it reads and writes config.json
//     and never opens the mirror; a mirror fact there would be noise.
func carriesFreshnessNotice(name string) bool {
	switch name {
	case toolQuery, toolSearch, toolIssue, toolShow, toolRecents, toolRetro:
		return true
	}
	return false
}

// freshnessNoticeSentence is what a tool's description says about the notice.
// toolDefinitions appends it to every tool carriesFreshnessNotice names and to
// no other, so the set an agent is told about and the set that emits it are the
// same list read twice (GDK-1813). Until then the extra content item was
// documented nowhere: an agent received a second, unannounced text item and had
// to guess whether it was part of the answer.
const freshnessNoticeSentence = `

A successful result may carry a second text item starting "Mirror freshness:" —
the local cache this answered from is behind, or its last sync failed. It is a
notice, not part of the payload (the first item is byte-identical either way):
read it, and say in your answer that the numbers are as of that point.`

// mirrorFreshnessLine is the notice for the mirror this server serves, or ""
// when the mirror is fresh (or its freshness cannot be read — silence never
// fails a tool; gadak_status and doctor own the detailed verdicts). It reuses
// the server's open handle when the tool already opened the mirror, and
// otherwise opens the file read-only for the check — gadak_show's keys/jql/
// issue paths intentionally never open the mirror for their own work.
func (s *Server) mirrorFreshnessLine() string {
	if s.db != nil {
		return freshness.Assess(s.db, time.Now(), s.confluenceConfigured).Notice()
	}
	ro, err := openReadOnly(s.DBPath)
	if err != nil {
		return ""
	}
	defer ro.Close()
	return freshness.Assess(ro, time.Now(), s.confluenceConfigured).Notice()
}

// confluenceConfigured is the lazy config answer freshness.Assess asks for
// only while a first sync is live on the issues source: whether the wiki
// pass is still to come. Profile-aware like every other config read here.
func (s *Server) confluenceConfigured() bool {
	cfg, err := config.LoadFor(s.Profile)
	return err == nil && cfg.Confluence != nil
}
