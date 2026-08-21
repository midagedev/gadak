package originbind

import (
	"errors"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestRefuseSiteRebindAllowsFirstBindAndSameSite(t *testing.T) {
	if err := RefuseSiteRebind(nil, "https://a.example"); err != nil {
		t.Fatalf("nil cfg: %v", err)
	}
	empty := &config.Config{}
	if err := RefuseSiteRebind(empty, "https://a.example"); err != nil {
		t.Fatalf("unbound workspace: %v", err)
	}
	bound := &config.Config{Site: "https://a.example/"}
	if err := RefuseSiteRebind(bound, "https://a.example"); err != nil {
		t.Fatalf("same site (slash/normalize): %v", err)
	}
	if err := RefuseSiteRebind(bound, "a.example"); err != nil {
		t.Fatalf("same site (scheme-less): %v", err)
	}
}

func TestRefuseSiteRebindNamesBoundSite(t *testing.T) {
	bound := &config.Config{Site: "https://old.example"}
	err := RefuseSiteRebind(bound, "https://new.example")
	if err == nil {
		t.Fatal("different site must be refused")
	}
	var refused *SiteBoundError
	if !errors.As(err, &refused) {
		t.Fatalf("err type %T, want *SiteBoundError", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "https://old.example") {
		t.Fatalf("bound site missing from %q", msg)
	}
	if !strings.Contains(msg, "gadak --workspace") || !strings.Contains(msg, "init") {
		t.Fatalf("want create-a-new-workspace sentence, got %q", msg)
	}
}
