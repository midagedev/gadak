package origin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
)

// The write-side half of GDK-1637: on a Jira Server origin every body field
// takes the typed characters as a JSON string — the doc the markdown paths
// built is not consulted — and on every other origin the doc goes as it
// always did.
func TestBodyValuePerOrigin(t *testing.T) {
	doc := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	wiki := "h2. Heading\n\n* one\n* two"
	cases := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"cloud", &config.Config{Kind: config.KindConnected, Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t"}, string(doc)},
		{"linear", &config.Config{Kind: config.KindConnected}, string(doc)}, // credential decides; Kind alone is pre-split
		{"built-in", &config.Config{Kind: config.KindStandalone}, string(doc)},
		{"server", &config.Config{Kind: config.OriginJiraServer, Site: "http://jira.example", Token: "pat"}, `"h2. Heading\n\n* one\n* two"`},
		{"nil config", nil, string(doc)},
	}
	for _, tc := range cases {
		got := string(BodyValue(tc.cfg, wiki, doc))
		if got != tc.want {
			t.Fatalf("%s: BodyValue = %s, want %s", tc.name, got, tc.want)
		}
	}
	// The Server branch ignores the doc entirely: a nil doc still yields the
	// string, so the description paths that skip the markdown conversion can
	// pass nothing.
	if got := string(BodyValue(cases[3].cfg, wiki, nil)); got != cases[3].want {
		t.Fatalf("server nil doc: %s", got)
	}
}

func TestBodyDialectPerOrigin(t *testing.T) {
	if BodyDialect(&config.Config{Kind: config.OriginJiraServer}) != adf.DialectWiki {
		t.Fatal("a Jira Server workspace is the wiki dialect")
	}
	for _, cfg := range []*config.Config{
		nil,
		{Kind: config.KindConnected, Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t"},
		{Kind: config.KindStandalone},
		{Kind: config.OriginLinear},
	} {
		if BodyDialect(cfg) != adf.DialectMarkdown {
			t.Fatalf("dialect of %+v must stay markdown", cfg)
		}
	}
}

// A wiki origin never refuses a placeholder-looking string: the characters
// are the body, verbatim — not markers standing for nodes of another body.
func TestRefuseBodyPlaceholdersPerOrigin(t *testing.T) {
	marker := "<!-- adf:1:0badf00d panel info --> note"
	if err := RefuseBodyPlaceholders(&config.Config{Kind: config.OriginJiraServer}, marker); err != nil {
		t.Fatalf("server origin refused verbatim text: %v", err)
	}
	err := RefuseBodyPlaceholders(&config.Config{Kind: config.KindConnected}, marker)
	if err == nil || !strings.Contains(err.Error(), "placeholders") {
		t.Fatalf("markdown origin must keep the refusal: %v", err)
	}
}
