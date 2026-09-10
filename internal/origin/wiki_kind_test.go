package origin

// GDK-1663: the wiki refusal is keyed by origin kind. A Jira Server
// workspace carries site+PAT and no email — that is a complete Server
// credential (GDK-1640), and the measured incident had `gadak page create`
// answer "site, email and token are required", sending the user hunting for
// an email their deployment does not have. The true reason is that this
// workspace has no wiki origin at all: Confluence Server is not implemented
// (its own issue tracks the dialect), and a Server site credential never
// implies one.

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestWikiRefusalNamesTheOriginKind(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want string
		ban  []string
	}{
		{
			// A complete Server credential — no email, by design.
			name: "server complete",
			cfg:  &config.Config{Kind: config.OriginJiraServer, Site: "https://jira.example.com", Token: "pat-token"},
			want: "no wiki origin",
			ban:  []string{"email", "token are required"},
		},
		{
			// Kind keys the sentence, not the credential fields: a Server
			// workspace that happens to carry an email must not be handed a
			// Cloud-shaped client for a Server site.
			name: "server with stray email",
			cfg:  &config.Config{Kind: config.OriginJiraServer, Site: "https://jira.example.com", Email: "a@b.c", Token: "pat-token"},
			want: "no wiki origin",
			ban:  []string{"token are required"},
		},
		{
			name: "linear",
			cfg:  &config.Config{Linear: &config.LinearConfig{APIKey: "lin-api"}},
			want: "no wiki origin",
			ban:  []string{"token are required", "Confluence"},
		},
		{
			// A half-configured Cloud workspace keeps the shared credential
			// sentence — for Cloud the missing email is the actual defect.
			name: "cloud half",
			cfg:  &config.Config{Site: "https://example.atlassian.net", Token: "t"},
			want: "site, email and token are required",
			ban:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Wiki(tc.cfg)
			if err == nil {
				t.Fatalf("Wiki must refuse this workspace, got a client")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal = %q, want it to say %q", err, tc.want)
			}
			for _, b := range tc.ban {
				if strings.Contains(err.Error(), b) {
					t.Fatalf("refusal %q must not say %q — that is another origin's story", err, b)
				}
			}
		})
	}
}
