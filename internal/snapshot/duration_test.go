package snapshot

import (
	"testing"
	"time"
)

func TestParseWindowComposite(t *testing.T) {
	// Go's parser handles compound units when no custom d/w suffix is used.
	d, err := ParseWindow("1h30m")
	if err != nil {
		t.Fatal(err)
	}
	if d != 90*time.Minute {
		t.Errorf("got %v", d)
	}
}
