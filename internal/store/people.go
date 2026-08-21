package store

import (
	"context"
	"unicode/utf8"
)

// Comment snippet length for the people-axis list (rune-safe for CJK).
const commentSnippetRunes = 160

// People-axis comment list defaults (GET people/{author_id}/comments/).
const (
	CommentsByAuthorDefaultLimit = 50
	CommentsByAuthorMaxLimit     = 200
)

// AuthorComment is one row in GET people/{author_id}/comments/.
type AuthorComment struct {
	Key       string `json:"key"`
	Kind      string `json:"kind"` // "issue" | "page"
	Title     string `json:"title"`
	Snippet   string `json:"snippet"`
	CreatedAt string `json:"created_at"`
}

// CommentsByAuthorResult is the response body for comments by author.
type CommentsByAuthorResult struct {
	// Author is the display name from the newest matching comment ("" when none).
	Author   string          `json:"author"`
	Total    int             `json:"total"`
	Comments []AuthorComment `json:"comments"`
}

// CommentsByAuthor returns comments for an exact author_id match, newest first.
// Missing author_id yields total 0 and an empty list (not an error). limit
// defaults to 50 and is capped at 200.
func (db *DB) CommentsByAuthor(ctx context.Context, authorID string, limit int) (CommentsByAuthorResult, error) {
	out := CommentsByAuthorResult{Comments: []AuthorComment{}}
	if authorID == "" {
		return out, nil
	}
	if limit <= 0 {
		limit = CommentsByAuthorDefaultLimit
	}
	if limit > CommentsByAuthorMaxLimit {
		limit = CommentsByAuthorMaxLimit
	}

	if err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE author_id = ?`, authorID,
	).Scan(&out.Total); err != nil {
		return out, err
	}
	if out.Total == 0 {
		return out, nil
	}

	rows, err := db.sql.QueryContext(ctx, `
		SELECT COALESCE(c.author, ''), COALESCE(it.key, ''), COALESCE(it.kind, ''),
		       COALESCE(it.title, ''), COALESCE(c.body_text, ''), COALESCE(c.body_adf, ''),
		       COALESCE(c.created_at, '')
		FROM comments c
		JOIN items it ON it.id = c.item_id
		WHERE c.author_id = ?
		ORDER BY c.created_at DESC
		LIMIT ?`, authorID, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var author, key, kind, title, bodyText, bodyADF, createdAt string
		if err := rows.Scan(&author, &key, &kind, &title, &bodyText, &bodyADF, &createdAt); err != nil {
			return out, err
		}
		if out.Author == "" {
			out.Author = author
		}
		out.Comments = append(out.Comments, AuthorComment{
			Key:       key,
			Kind:      kind,
			Title:     title,
			Snippet:   commentSnippet(bodyText, bodyADF),
			CreatedAt: createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// commentSnippet builds a list snippet: body_text when present, else ADF plain
// text; whitespace-normalized; hard-cut at commentSnippetRunes (no ellipsis).
func commentSnippet(bodyText, bodyADF string) string {
	text := bodyText
	if text == "" {
		text = plainExcerptFromADF(bodyADF)
	}
	text = normalizeWhitespace(text)
	if text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) <= commentSnippetRunes {
		return text
	}
	return string([]rune(text)[:commentSnippetRunes])
}

// UserCatalog reads the cached origin account catalog (GDK-590): every
// account sync has seen, with the origin's account_type spelling. Callers
// judge bots through the connector's one judgement function, never here —
// the store is source-neutral.
func (db *DB) UserCatalog(ctx context.Context) ([]UserAccount, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT account_id, COALESCE(name, ''), COALESCE(email, ''), COALESCE(account_type, '')
		FROM users ORDER BY source_id, account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserAccount{}
	for rows.Next() {
		var u UserAccount
		if err := rows.Scan(&u.AccountID, &u.Name, &u.Email, &u.AccountType); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// IssueActor is one touch of one issue by one account: a comment, a
// changelog entry, or a development-panel link (GDK-590). The union is also
// the issue_actors view, so `gadak sql` and documented recipes see the same
// axis the server builds members from.
type IssueActor struct {
	IssueKey   string
	SourceID   string
	ActorID    string
	ActorName  string
	Via        string // "comment" | "changelog" | "dev_link"
}

// QueryIssueActors returns every (issue, actor) touch, unordered. The set is
// small — bounded by comments + changelog + dev_links rows — and the caller
// (buildView) only folds it into a map, so no LIMIT.
func (db *DB) QueryIssueActors(ctx context.Context) ([]IssueActor, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT issue_key, source_id, actor_id, COALESCE(actor_name, ''), via FROM issue_actors`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IssueActor{}
	for rows.Next() {
		var a IssueActor
		if err := rows.Scan(&a.IssueKey, &a.SourceID, &a.ActorID, &a.ActorName, &a.Via); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
