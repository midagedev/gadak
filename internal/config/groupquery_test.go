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
