package store

import (
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
func (db *DB) CommentsByAuthor(authorID string, limit int) (CommentsByAuthorResult, error) {
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

	if err := db.sql.QueryRow(
		`SELECT COUNT(*) FROM comments WHERE author_id = ?`, authorID,
	).Scan(&out.Total); err != nil {
		return out, err
	}
	if out.Total == 0 {
		return out, nil
	}

	rows, err := db.sql.Query(`
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
