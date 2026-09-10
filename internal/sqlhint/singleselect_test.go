package sqlhint

import "testing"

func TestClassifySingleSelect(t *testing.T) {
	cases := []struct {
		q    string
		want SingleSelectVerdict
		kw   string
	}{
		{"SELECT 1", SingleSelectOK, ""},
		{"with x as (select 1) select * from x", SingleSelectOK, ""},
		{"-- comment\nSELECT 1", SingleSelectOK, ""},
		{"/* block */ SELECT 1", SingleSelectOK, ""},
		{"SELECT 1;", SingleSelectOK, ""},
		{"SELECT/*x*/1", SingleSelectOK, ""},
		{"", SingleSelectEmpty, ""},
		{"-- only a comment", SingleSelectEmpty, ""},
		{"SELECT 1; SELECT 2", SingleSelectMultiStatement, ""},
		{"INSERT INTO t VALUES (1)", SingleSelectOtherKeyword, "INSERT"},
		{"pragma table_info(issues)", SingleSelectOtherKeyword, "pragma"},
		{";", SingleSelectNoKeyword, ""},
	}
	for _, c := range cases {
		got, kw := ClassifySingleSelect(c.q)
		if got != c.want || kw != c.kw {
			t.Errorf("ClassifySingleSelect(%q) = (%v, %q), want (%v, %q)", c.q, got, kw, c.want, c.kw)
		}
	}
}
