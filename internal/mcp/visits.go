package mcp

// visits.go — the single owner of "an MCP read leaves a trail".
//
// GDK-502 made `gadak issue` and `gadak search` append the personal-history
// rows that `gadak recents` walks back, at the point where the load
// succeeded. That landed in package main, so the MCP surface — the only path
// a shell-less host (Claude Desktop) has — kept reading the mirror without
// recording anything: an agent that works entirely through these tools had
// zero rows, and after a compaction recents was empty (GDK-631).
//
// The fix is not a call bolted onto each tool. s.db.Detail and s.db.Search
// are private to this file: every read path in the package goes through
// loadDetail / runSearch, so a new tool cannot reach the mirror's detail or
// search without inheriting the recording. TestMirrorReadsGoThroughTheVisit
// Recorder parses the package and asserts exactly that.
//
// Recording is best-effort, the same contract cmd/gadak/agent_issue.go states:
// reading is the command and history is a side effect, so a local.db that
// cannot take the row must not fail the tool call — one stderr line, the
// result untouched. Neither warning echoes its payload: the query text stays
// out of the process log here as it does on the CLI and in the server.
//
// Duplication note: recordVisitBestEffort / recordSearchBestEffort live in
// package main (cmd/gadak) and cannot be imported. The two functions below
// are a deliberate second implementation of those four lines; what is NOT
// duplicated is the policy they encode — store.RecordVisit/RecordSearch is
// the shared owner, and store.VisitSourceMCP already existed for this caller.

import (
	"context"

	"github.com/midagedev/gadak/internal/store"
)

// loadDetail is the package's only reader of one issue's detail: the load
// point, so every caller (one key, several keys, a future tool) records.
func (s *Server) loadDetail(ctx context.Context, key string) (*store.Detail, error) {
	d, err := s.db.Detail(ctx, key)
	if err != nil {
		// A key the mirror does not have is not a visit — the CLI's notFound
		// keys never reach its recorder either.
		return nil, err
	}
	s.recordVisitBestEffort(ctx, store.VisitKindIssue, key)
	return d, nil
}

// runSearch is the package's only caller of the mirror's search.
func (s *Server) runSearch(ctx context.Context, text string, limit int) (store.SearchResult, error) {
	res, err := s.db.Search(ctx, text, limit)
	if err != nil {
		return res, err
	}
	// res.Total is the count this tool's own answer reports — the same number
	// the CLI and the web client record for the same search.
	s.recordSearchBestEffort(ctx, text, res.Total)
	return res, nil
}

func (s *Server) recordVisitBestEffort(ctx context.Context, kind, key string) {
	if _, err := s.db.RecordVisit(ctx, kind, key, store.VisitSourceMCP); err != nil {
		Logf("could not record this visit in local history: %v", err)
	}
}

func (s *Server) recordSearchBestEffort(ctx context.Context, query string, resultCount int) {
	if _, err := s.db.RecordSearch(ctx, query, resultCount, "", ""); err != nil {
		Logf("could not record this search in local history: %v", err)
	}
}
