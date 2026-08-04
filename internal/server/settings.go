package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/midagedev/scry/internal/config"
)

// The client falls back to 72 hours when the document omits the threshold, but a
// literal 0 would override that and mark every issue stale, so the document
// always carries a real number.
const defaultStaleHours = 72

// Every optional surface a config flag can switch on. Unknown keys in the
// configuration are dropped: a flag nobody reads is a flag that does nothing.
var featureNames = []string{"presence", "feed", "push", "deploy", "qa", "teamGroups"}

type webConfigDoc struct {
	APIBase             string                    `json:"apiBase"`
	AuthBase            string                    `json:"authBase"`
	JiraBaseURL         string                    `json:"jiraBaseUrl"`
	QaDashboardURL      string                    `json:"qaDashboardUrl"`
	Projects            []string                  `json:"projects"`
	GroupLabels         map[string]string         `json:"groupLabels"`
	GroupColors         map[string]string         `json:"groupColors"`
	ProductByGroup      map[string]config.Product `json:"productByGroup"`
	StaleThresholdHours int                       `json:"staleThresholdHours"`
	Features            map[string]bool           `json:"features"`
}

// webConfig is the credential-free projection of the configuration. Site is the
// only field of the credential block that appears here, as a deep-link base.
func webConfig(cfg *config.Config) webConfigDoc {
	if cfg == nil {
		cfg = &config.Config{}
	}
	stale := cfg.StaleThresholdHours
	if stale <= 0 {
		stale = defaultStaleHours
	}
	return webConfigDoc{
		APIBase:             apiBase,
		AuthBase:            authBase,
		JiraBaseURL:         strings.TrimRight(cfg.Site, "/"),
		QaDashboardURL:      cfg.QaDashboardURL,
		Projects:            strs(cfg.Projects),
		GroupLabels:         strMap(cfg.GroupLabels),
		GroupColors:         strMap(cfg.GroupColors),
		ProductByGroup:      products(cfg.ProductByGroup),
		StaleThresholdHours: stale,
		Features:            features(cfg.Features),
	}
}

// settingsDoc is everything the settings UI may read and write. The credential
// block (email, token) is absent by construction, not by filtering.
type settingsDoc struct {
	Projects            []string                  `json:"projects"`
	FieldMap            map[string]string         `json:"fieldMap"`
	BodyFields          []string                  `json:"bodyFields"`
	EditableFields      map[string]string         `json:"editableFields"`
	Members             []config.Member           `json:"members"`
	GroupRules          []config.GroupRule        `json:"groupRules"`
	GroupLabels         map[string]string         `json:"groupLabels"`
	GroupColors         map[string]string         `json:"groupColors"`
	ProductByGroup      map[string]config.Product `json:"productByGroup"`
	Features            map[string]bool           `json:"features"`
	QaDashboardURL      string                    `json:"qaDashboardUrl"`
	StaleThresholdHours int                       `json:"staleThresholdHours"`

	// Read-only context for the UI. Ignored on PUT — the site and the token are
	// the credential endpoint's business (T4).
	Site          string `json:"site"`
	HasCredential bool   `json:"hasCredential"`
}

func (s *server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, settings(s.config()))
}

func (s *server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsDoc
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	// Copy the live config so the credential block and the sync intervals survive
	// a settings write untouched.
	next := *s.config()
	next.Projects = in.Projects
	next.FieldMap = in.FieldMap
	next.BodyFields = in.BodyFields
	next.EditableFields = in.EditableFields
	next.Members = in.Members
	next.GroupRules = in.GroupRules
	next.GroupLabels = in.GroupLabels
	next.GroupColors = in.GroupColors
	next.ProductByGroup = in.ProductByGroup
	next.Features = features(in.Features)
	next.QaDashboardURL = in.QaDashboardURL
	next.StaleThresholdHours = max(in.StaleThresholdHours, 0)

	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	// Members and group rules feed the cached projection; it has to be rebuilt.
	s.gen.Add(1)
	writeJSON(w, http.StatusOK, settings(&next))
}

func settings(cfg *config.Config) settingsDoc {
	return settingsDoc{
		Projects:            strs(cfg.Projects),
		FieldMap:            strMap(cfg.FieldMap),
		BodyFields:          strs(cfg.BodyFields),
		EditableFields:      strMap(cfg.EditableFields),
		Members:             members(cfg.Members),
		GroupRules:          rules(cfg.GroupRules),
		GroupLabels:         strMap(cfg.GroupLabels),
		GroupColors:         strMap(cfg.GroupColors),
		ProductByGroup:      products(cfg.ProductByGroup),
		Features:            features(cfg.Features),
		QaDashboardURL:      cfg.QaDashboardURL,
		StaleThresholdHours: cfg.StaleThresholdHours,
		Site:                cfg.Site,
		HasCredential:       cfg.HasCredential(),
	}
}

// The helpers below only replace nil with empty, so the documents carry `[]` and
// `{}` instead of `null` and the UI can bind to them without guards.

func features(set map[string]bool) map[string]bool {
	out := make(map[string]bool, len(featureNames))
	for _, name := range featureNames {
		out[name] = set[name]
	}
	return out
}

func strs(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func strMap(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}

func products(v map[string]config.Product) map[string]config.Product {
	if v == nil {
		return map[string]config.Product{}
	}
	return v
}

func members(v []config.Member) []config.Member {
	if v == nil {
		return []config.Member{}
	}
	return v
}

func rules(v []config.GroupRule) []config.GroupRule {
	if v == nil {
		return []config.GroupRule{}
	}
	return v
}
