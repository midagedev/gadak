package origin

import (
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairing"
)

/*
 * GDK-1152: the capability table. Each row is one origin type × transport as
 * the product can actually construct it, with the capabilities the origin
 * itself must state. This table is the documentation — the cost of a new
 * workspace kind is a row here, not an audit of every surface.
 *
 * Rows follow the real constructors: gadak+local is the built-in workspace
 * (Kind standalone), gadak+remote is a paired one (remote-origin.json), and
 * the Jira family and Linear only ever answer across a serve API.
 */
func TestCapabilitiesTable(t *testing.T) {
	paired := func(t *testing.T) *config.Config {
		t.Helper()
		t.Setenv("GADAK_HOME", t.TempDir())
		config.SetProfile("")
		t.Cleanup(func() { config.SetProfile("") })
		cfg, err := config.LoadFor("")
		if err != nil {
			t.Fatal(err)
		}
		if err := pairing.SaveRemote(cfg.Directory(), pairing.Remote{
			Endpoint: "https://home.example.com:8443",
			Token:    "device-token",
			Label:    "laptop",
		}); err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	cases := []struct {
		name string
		cfg  *config.Config
		want Capabilities
	}{
		{
			// Nothing configured: the Jira-family defaults, no credential.
			// credentialRequired stays true — the empty workspace's way in
			// is exactly the site-token dialog/CTA (as today: the footer CTA
			// shows because originWritable is false).
			name: "unconfigured (jira × remote)",
			cfg:  nil,
			want: Capabilities{CredentialRequired: true},
		},
		{
			// A site named but no token: links build (browse is public),
			// nothing writes, and identity is absent — auth/me gates on
			// HasCredential before answering cfg.Email.
			name: "jira cloud site only (jira × remote)",
			cfg:  &config.Config{Site: "https://x.example/"},
			want: Capabilities{
				OriginDeepLink:     true,
				OriginBaseURL:      "https://x.example",
				CredentialRequired: true,
			},
		},
		{
			name: "jira cloud with credential (jira × remote)",
			cfg: &config.Config{
				Site: "https://x.example", Email: "a@b.example", Token: "t",
			},
			want: Capabilities{
				IssueWrite:         true,
				WikiWrite:          true,
				Identity:           true,
				OriginDeepLink:     true,
				OriginBaseURL:      "https://x.example",
				CredentialRequired: true,
			},
		},
		{
			// jira-server: site+PAT writes issues, has no Confluence
			// surface, and the PAT is a site token (GDK-1640).
			name: "jira server with PAT (jira-server × remote)",
			cfg:  &config.Config{Kind: config.OriginJiraServer, Site: "https://j.example", Token: "pat"},
			want: Capabilities{
				IssueWrite:         true,
				OriginDeepLink:     true,
				OriginBaseURL:      "https://j.example",
				CredentialRequired: true,
			},
		},
		{
			// Linear: writes ride credential.linear per key (issueWrite is
			// the Jira-family axis by contract — see Capabilities.IssueWrite),
			// there is no wiki surface, and the link is the per-issue URL
			// the mirror stores, never a base the client could build from.
			name: "linear with api key (linear × remote)",
			cfg:  &config.Config{Linear: &config.LinearConfig{APIKey: "lin_api"}},
			want: Capabilities{OriginDeepLink: true},
		},
		{
			// Built-in: issuetap answers everything in-process, has no
			// identity to answer auth/me with, and is its own page — so no
			// origin deep link and no credential to require.
			name: "built-in (gadak × local)",
			cfg:  &config.Config{Kind: config.KindStandalone},
			want: Capabilities{IssueWrite: true, WikiWrite: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CapabilitiesOf(tc.cfg)
			if got != tc.want {
				t.Fatalf("capabilities = %+v, want %+v", got, tc.want)
			}
		})
	}

	// The paired row needs its own config construction, so it runs outside
	// the table loop: same shape as built-in (issuetap answers issue and
	// wiki writes), but reached across a serve API — the row whose SyncTab
	// residual named this axis (GDK-1148).
	t.Run("paired (gadak × remote)", func(t *testing.T) {
		got := CapabilitiesOf(paired(t))
		want := Capabilities{IssueWrite: true, WikiWrite: true}
		if got != want {
			t.Fatalf("paired capabilities = %+v, want %+v", got, want)
		}
	})
}
