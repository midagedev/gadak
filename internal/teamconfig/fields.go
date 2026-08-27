package teamconfig

// Shared team settings: Config struct field names that export may include.
// Members is exportable only when the caller opts in (--with-members); it still
// belongs on this list so the reflection coverage test forces an explicit
// decision when Config grows.
//
// Keep this list as the single source of "what is team-shareable".
var exportableConfigFields = []string{
	"Projects",
	"Fields",
	"FieldMap",
	"BodyFields",
	"EditableFields",
	"Members",
	"GroupRules",
	"GroupQuery",
	"GroupLabels",
	"GroupColors",
	"ProductByGroup",
	"Features",
	"QaDashboardURL",
	"StaleThresholdHours",
	"Confluence",
}

// Config struct field names that must never appear in a team file. Secrets,
// personal identity, and per-machine preferences belong here — not a
// blacklist of "things we thought of today", but the complement of the
// whitelist above. The reflection test requires every Config field to land in
// exactly one of the two lists.
var neverExportConfigFields = []string{
	"Site",
	"Email",
	"Token",
	"TokenVerifiedAt",
	"TokenOwner",
	"TokenExpiresAt",
	"TokenExpirySource",
	"AccountID",
	"AttachmentCacheMB",
	"SyncIntervalSec",
	"ReconcileIntervalSec",
	"Notify",
	"UpdateCheck",
	"Appearance",
	// Default project/type are site- and project-bound ids (plus an optional
	// display label). Another account's createmeta will not share those type
	// ids, so exporting them would file as the wrong type or fail. Personal,
	// not team consensus.
	"DefaultProject",
	"DefaultIssueTypeID",
	"DefaultIssueType",
	// Workspace kind is per-machine: standalone origin is this profile's
	// issuetap snapshot, not a team setting.
	"Kind",
	// Frozen is a per-workspace safety latch (GDK-181): a scrubbed fixture
	// with a live credential. Exporting it would freeze someone else's
	// real mirror, or un-freeze a demo the importer did not mean to open.
	"Frozen",
	// DevStatus opts one workspace into per-issue dev-status calls against
	// Jira's internal API (GDK-496) — a per-machine cost/risk choice, not
	// team consensus.
	"DevStatus",
	// Linear carries a personal API key. Team scope would be shareable, but
	// splitting the block to export half of it is not worth a credential
	// classification mistake — the whole block stays private.
	"Linear",
	// FieldsAppliedAt is when this machine last ran `gadak fields --apply`.
	// The specs themselves are team-shareable (Fields); the timestamp is not.
	"FieldsAppliedAt",
	// Actor is this machine's agent identity for issuetap-bound writes
	// (GDK-586). Whose shell is running is not team consensus, and a shared
	// slug would merge every importer's agent into one origin account.
	"Actor",
	// Locale names the language of this profile's standalone origin snapshot
	// (GDK-597) — the same per-machine character as Kind: the origin the
	// setting describes is not part of a team file, and language is the
	// importer's own preference. A connected workspace ignores it anyway.
	"Locale",
	// UI is this machine's color overrides (GDK-786): personal aesthetics in
	// the same seat as Appearance, and its dataColors.type keys are site-bound
	// issue_type ids — another account would tint the wrong types.
	"UI",
	// Terminal behavior (GDK-896) names machine-local paths — a shell and a
	// starting directory that exist on this machine. Same seat as UI: a
	// personal preference, not team consensus.
	"Terminal",
}

// credentialJSONKeys are JSON object keys that mean "this file carries
// credentials or personal identity". Import refuses any document that has one
// of these at the top level or inside "settings" — even if a human added them
// by hand. Names match Config json tags.
var credentialJSONKeys = map[string]bool{
	"site":              true,
	"email":             true,
	"token":             true,
	"tokenVerifiedAt":   true,
	"tokenOwner":        true,
	"tokenExpiresAt":    true,
	"tokenExpirySource": true,
	"account_id":        true,
	"linear":            true, // carries apiKey — a Linear personal API key
}
