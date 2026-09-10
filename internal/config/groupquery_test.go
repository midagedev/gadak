package config

import "testing"

func TestValidateGroupQuery(t *testing.T) {
	ok := []string{
		"",
		"SELECT key, 'g' FROM issues_full",
		"WITH x AS (SELECT key FROM issues) SELECT key, NULL FROM x",
		"-- comment\nSELECT key, '' FROM issues",
	}
	for _, q := range ok {
		if err := ValidateGroupQuery(q); err != nil {
			t.Errorf("ValidateGroupQuery(%q) = %v", q, err)
		}
	}
	bad := []string{
		"DELETE FROM issues",
		"SELECT 1; SELECT 2",
		"PRAGMA table_info(issues)",
		"ATTACH 'x.db' AS x",
	}
	for _, q := range bad {
		if err := ValidateGroupQuery(q); err == nil {
			t.Errorf("ValidateGroupQuery(%q) = nil, want error", q)
		}
	}
}

// Two comment-stripping edges the groupQuery gate shares with sqlhint/MCP.
// Pre-fix copies in this package: (a) skipped double-quoted identifiers, so a
// "--" inside "…" hid the next statement; (b) block comments glued tokens
// (SELECT/*x*/key → SELECTkey). SELECT/*x*/1 stays a SELECT even without the
// space (the glued '1' is not an identifier character); the letter-glue form
// is the public-gate manifestation of the same defect.
func TestValidateGroupQueryCommentEdges(t *testing.T) {
	if err := ValidateGroupQuery("SELECT/*x*/1"); err != nil {
		t.Errorf("SELECT/*x*/1 must stay a SELECT, got %v", err)
	}
	if err := ValidateGroupQuery("SELECT/*x*/key FROM issues_full"); err != nil {
		t.Errorf("SELECT/*x*/key must stay a SELECT, got %v", err)
	}
	q := `SELECT 1 FROM "col--name"; DELETE FROM issues`
	if err := ValidateGroupQuery(q); err == nil {
		t.Errorf("double-quoted -- must not hide a second statement: %q", q)
	}
}

// The settings surface's own rejection sentences, kept distinct from the MCP
// tool's (GDK-928). Empty input stays accepted: that is how the group query is
// disabled.
func TestValidateGroupQueryMessages(t *testing.T) {
	if err := ValidateGroupQuery("   "); err != nil {
		t.Errorf("blank groupQuery = %v, want nil (disabled)", err)
	}
	cases := []struct{ q, want string }{
		{"-- only a comment", "groupQuery is empty after comments"},
		{"SELECT 1; SELECT 2", "groupQuery must be one SELECT or WITH"},
		{"INSERT INTO t VALUES (1)", `groupQuery must be SELECT or WITH (got "INSERT")`},
	}
	for _, c := range cases {
		err := ValidateGroupQuery(c.q)
		if err == nil || err.Error() != c.want {
			t.Errorf("ValidateGroupQuery(%q) = %v, want %q", c.q, err, c.want)
		}
	}
}
