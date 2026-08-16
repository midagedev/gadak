package config

import "testing"

// Pins the on-the-wire spelling. Other packages must import ISOMilli rather
// than restating this layout; a silent edit of the constant must fail here.
func TestISOMilliLayout(t *testing.T) {
	t.Parallel()
	const want = "2006-01-02T15:04:05.000Z"
	if ISOMilli != want {
		t.Fatalf("ISOMilli = %q, want %q", ISOMilli, want)
	}
}
