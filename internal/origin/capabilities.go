package origin

import (
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

// Capabilities is the origin's own statement of what it can do — the block
// config.json carries so no surface has to guess it from the workspace kind,
// an empty site URL, or auth/me's identity (GDK-1152). The class of defect
// this closes: every new workspace kind used to cost a full audit of every
// surface, because each re-derived "can this origin accept X" from whatever
// signal was nearby. With this block the cost is one row in the table test
// (capabilities_table_test.go — that table is the documentation).
//
// Every value is owned by a predicate that already existed — the same ones
// the write paths and auth/me answer from. Nothing here introduces a new
// judgment about the workspace; it only names the questions.
type Capabilities struct {
	// IssueWrite is exactly config.HasAtlassianCredential — the Jira-family
	// write predicate the web already mirrors as `originWritable` (GDK-1090):
	// built-in, a jira-server site+PAT, a cloud site+email+token, or a
	// pairing remote. Linear is deliberately not counted: a Linear key rides
	// its own per-key path (credential.linear), which the web's write gate
	// already asks per issue. The alias is pinned by test — the wire field
	// and this axis must not drift.
	IssueWrite bool `json:"issueWrite"`
	// WikiWrite is whether origin.Wiki constructs — page writes (create,
	// edit, comment; GDK-380/381/382) can reach the origin: paired or
	// in-process issuetap always; a cloud site needs the full credential;
	// jira-server has no Confluence surface (errNoWikiServer) and Linear
	// none at all (errNoWikiOrigin). The Confluence *source* being switched
	// on is a separate axis (confluenceEnabled): this says the write path
	// exists, not that pages are mirrored.
	WikiWrite bool `json:"wikiWrite"`
	// Identity is what auth/me answers from — a credential plus a non-empty
	// cfg.Email. False on built-in and paired workspaces by construction
	// (no email to answer with) even though both write fine; that split is
	// the whole point of this block (GDK-1090's near-miss).
	Identity bool `json:"identity"`
	// OriginDeepLink is whether issues have a page on the origin the client
	// can link to. Jira family: the site's /browse/KEY (OriginBaseURL
	// carries the site). Linear: true with an empty base — a Linear
	// workspace has no site; the link is the per-issue URL the mirror
	// stores (web lib/issue-origin.ts, GDK-1308). Gadak's own tracker:
	// false — its /browse/KEY is this app, not an origin page.
	OriginDeepLink bool `json:"originDeepLink"`
	// OriginBaseURL is the site an origin page URL is built from — empty
	// except on the Jira family. Never a credential; the browse page is
	// public regardless of token state.
	OriginBaseURL string `json:"originBaseUrl"`
	// CredentialRequired is whether this workspace's write credential is a
	// site token the app's credential surface (the personal-token dialog,
	// PUT credential) can edit. True only on the Jira family reached as a
	// site: a built-in writes through its in-process origin, a paired one
	// keeps its credential in remote-origin.json on the home machine, and a
	// Linear key is config, not a site token. This is the axis the SyncTab
	// entry point needed (GDK-1148's paired residual): advising any of
	// those three to "set credentials" sells a concept that does not exist
	// there, while hiding it on a connected workspace would take away the
	// only in-app way to rotate a real token.
	CredentialRequired bool `json:"credentialRequired"`
}

// CapabilitiesOf states cfg's origin capabilities. Nil-safe like Describe.
// The branch order mirrors Wiki()'s own (paired, then built-in, then the
// kind ladder) so the bool cannot drift from the constructor it summarizes.
func CapabilitiesOf(cfg *config.Config) Capabilities {
	if cfg == nil {
		cfg = &config.Config{}
	}
	caps := Capabilities{
		IssueWrite: cfg.HasAtlassianCredential(),
		Identity:   cfg.HasCredential() && cfg.Email != "",
	}
	rem, err := pairedRemote(cfg)
	paired := err == nil && rem != nil
	switch {
	case paired || cfg.HasBuiltInOrigin():
		// issuetap answers issue and wiki writes either way it is reached;
		// its tracker is this app, so no origin page and no site token.
		caps.WikiWrite = true
	case cfg.OriginType() == config.OriginLinear:
		// errNoWikiOrigin: no Confluence surface, and the Linear key is not
		// a site credential any dialog here could edit.
		caps.OriginDeepLink = true
	case cfg.OriginType() == config.OriginJiraServer:
		// errNoWikiServer: Confluence is Cloud's. The PAT is a site token.
		caps.CredentialRequired = true
		caps.OriginDeepLink = cfg.Site != ""
		caps.OriginBaseURL = strings.TrimRight(cfg.Site, "/")
	default:
		// Jira Cloud: wiki writes need the full site credential; issue
		// writes and the token dialog are HasAtlassianCredential's ladder.
		caps.WikiWrite = cfg.Site != "" && cfg.Email != "" && cfg.Token != ""
		caps.CredentialRequired = true
		caps.OriginDeepLink = cfg.Site != ""
		caps.OriginBaseURL = strings.TrimRight(cfg.Site, "/")
	}
	return caps
}
