package store

import "testing"

// The contract the two former implementations disagreed on (GDK-927): units
// above MB, and negative sizes. The CLI copy printed "1024.0 MB" for a
// gigabyte and "-1 B" for a negative count; these rows are the reason the
// merge kept the server's implementation.
func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "0 B"},
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1 << 20, "1.0 MB"},
		{1 << 30, "1.0 GB"},
		{1 << 40, "1.0 TB"},
	}
	for _, tc := range cases {
		if got := HumanBytes(tc.n); got != tc.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
