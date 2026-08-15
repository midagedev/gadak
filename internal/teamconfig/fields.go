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
}
