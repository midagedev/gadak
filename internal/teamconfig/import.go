package teamconfig

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// ImportOptions controls merge behaviour.
type ImportOptions struct {
	// Overwrite replaces conflicting settings keys and same-named views.
	Overwrite bool
	// DryRun builds and returns a plan without applying it.
	DryRun bool
}

// SettingAction describes what happens to one settings key.
type SettingAction string

const (
	SettingAdd     SettingAction = "add"
	SettingSkip    SettingAction = "skip"
	SettingReplace SettingAction = "replace"
)

// ViewAction describes what happens to one named view.
type ViewAction string

const (
	ViewAdd     ViewAction = "add"
	ViewSkip    ViewAction = "skip"
	ViewReplace ViewAction = "replace"
)

// SettingChange is one planned settings field update.
type SettingChange struct {
	Key    string // JSON key (e.g. "fields")
	Action SettingAction
}

// ViewChange is one planned saved-view update.
type ViewChange struct {
	Name   string
	Action ViewAction
	// ExistingID is set when Action is ViewReplace (keep machine-local id).
	ExistingID string
	// Config is the incoming view config (for add/replace).
	Config json.RawMessage
}

// Plan is a pure value describing what import would do. Dry-run prints it;
// real import applies the same plan so the two paths cannot diverge.
type Plan struct {
	Settings []SettingChange
	Views    []ViewChange
	// Incoming is the validated document (settings + views to apply from).
	Incoming Document
}

// Summary lines for human output (stable order).
func (p Plan) SummaryLines() []string {
	var lines []string
	var add, skip, replace []string
	for _, s := range p.Settings {
		switch s.Action {
		case SettingAdd:
			add = append(add, s.Key)
		case SettingSkip:
			skip = append(skip, s.Key)
		case SettingReplace:
			replace = append(replace, s.Key)
		}
	}
	if len(add) > 0 {
		lines = append(lines, "settings add: "+strings.Join(add, ", "))
	}
	if len(replace) > 0 {
		lines = append(lines, "settings replace: "+strings.Join(replace, ", "))
	}
	if len(skip) > 0 {
		lines = append(lines, "settings skip (already set): "+strings.Join(skip, ", "))
	}
	if len(add)+len(skip)+len(replace) == 0 {
		lines = append(lines, "settings: nothing to change")
	}

	var vAdd, vSkip, vReplace []string
	for _, v := range p.Views {
		switch v.Action {
		case ViewAdd:
			vAdd = append(vAdd, v.Name)
		case ViewSkip:
			vSkip = append(vSkip, v.Name)
		case ViewReplace:
			vReplace = append(vReplace, v.Name)
		}
	}
	if len(vAdd) > 0 {
		lines = append(lines, "views add: "+strings.Join(vAdd, ", "))
	}
	if len(vReplace) > 0 {
		lines = append(lines, "views replace: "+strings.Join(vReplace, ", "))
	}
	if len(vSkip) > 0 {
		lines = append(lines, "views skip (same name exists): "+strings.Join(vSkip, ", "))
	}
	if len(vAdd)+len(vSkip)+len(vReplace) == 0 {
		lines = append(lines, "views: nothing to change")
	}
	return lines
}

// ParseDocument unmarshals raw team-config JSON, rejects unknown versions and
// any credential-shaped keys (site/email/token/…).
func ParseDocument(raw []byte) (Document, error) {
	if err := rejectCredentialKeys(raw); err != nil {
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, fmt.Errorf("invalid team config JSON: %w", err)
	}
	if doc.Version == 0 {
		// Files written before the 2026-08 rename used scry_team_config.
		var legacy struct {
			Version int `json:"scry_team_config"`
		}
		_ = json.Unmarshal(raw, &legacy)
		doc.Version = legacy.Version
	}
	if doc.Version == 0 {
		return Document{}, fmt.Errorf("missing required field gadak_team_config (version)")
	}
	if doc.Version != CurrentFormat {
		return Document{}, fmt.Errorf("unsupported gadak_team_config version %d (this gadak understands %d)", doc.Version, CurrentFormat)
	}
	// Normalize empty view configs.
	for i := range doc.Views {
		if len(doc.Views[i].Config) == 0 {
			doc.Views[i].Config = json.RawMessage("{}")
		}
		if strings.TrimSpace(doc.Views[i].Name) == "" {
			return Document{}, fmt.Errorf("views[%d]: name is required", i)
		}
	}
	normalizeLegacySettings(&doc.Settings)
	return doc, nil
}

// normalizeLegacySettings converts leftover fieldMap/editableFields into
// Fields using the same synthesis LoadFor persists, then drops the old keys
// so merge is Fields-vs-Fields.
func normalizeLegacySettings(s *TeamSettings) {
	if s == nil {
		return
	}
	tmp := config.Config{
		Fields:         copyFieldSpecs(s.Fields),
		FieldMap:       copyStringMap(s.FieldMap),
		EditableFields: copyStringMap(s.EditableFields),
	}
	if changed, _ := tmp.NormalizeLegacyFields(); !changed {
		return
	}
	s.Fields = tmp.Fields
	s.FieldMap = nil
	s.EditableFields = nil
}

// rejectCredentialKeys walks top-level and settings object keys. If any
// credential-related key is present, import fails loudly so the file is not
// treated as safe to share.
func rejectCredentialKeys(raw []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return fmt.Errorf("invalid team config JSON: %w", err)
	}
	var found []string
	for k := range top {
		if credentialJSONKeys[k] {
			found = append(found, k)
		}
	}
	if settingsRaw, ok := top["settings"]; ok && len(settingsRaw) > 0 && string(settingsRaw) != "null" {
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(settingsRaw, &settings); err == nil {
			for k := range settings {
				if credentialJSONKeys[k] {
					found = append(found, "settings."+k)
				}
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return fmt.Errorf("this file contains credentials or personal identity keys (%s) — do not share it; remove those keys and use a file produced by `gadak team export`",
		strings.Join(found, ", "))
}

// BuildPlan compares the incoming document to the current config and views.
func BuildPlan(current *config.Config, existingViews []store.SavedView, doc Document, opts ImportOptions) Plan {
	if current == nil {
		current = &config.Config{}
	}
	p := Plan{Incoming: doc}
	p.Settings = planSettings(current, doc.Settings, opts.Overwrite)

	byName := make(map[string]store.SavedView, len(existingViews))
	for _, v := range existingViews {
		byName[v.Name] = v
	}
	for _, iv := range doc.Views {
		if ex, ok := byName[iv.Name]; ok {
			if opts.Overwrite {
				p.Views = append(p.Views, ViewChange{
					Name:       iv.Name,
					Action:     ViewReplace,
					ExistingID: ex.ID,
					Config:     iv.Config,
				})
			} else {
				p.Views = append(p.Views, ViewChange{
					Name:   iv.Name,
					Action: ViewSkip,
				})
			}
			continue
		}
		p.Views = append(p.Views, ViewChange{
			Name:   iv.Name,
			Action: ViewAdd,
			Config: iv.Config,
		})
	}
	return p
}

func planSettings(cur *config.Config, in TeamSettings, overwrite bool) []SettingChange {
	type item struct {
		key   string
		empty bool // current is empty (can fill without overwrite)
		hasIn bool // incoming has a non-empty value
		apply func()
	}
	// apply is unused in plan; ApplyPlan uses setSettingByKey.
	items := []item{
		{key: "projects", empty: len(cur.Projects) == 0, hasIn: len(in.Projects) > 0},
		{key: "fields", empty: len(cur.Fields) == 0, hasIn: len(in.Fields) > 0},
		{key: "bodyFields", empty: len(cur.BodyFields) == 0, hasIn: len(in.BodyFields) > 0},
		{key: "members", empty: len(cur.Members) == 0, hasIn: len(in.Members) > 0},
		{key: "groupRules", empty: len(cur.GroupRules) == 0, hasIn: len(in.GroupRules) > 0},
		{key: "groupQuery", empty: cur.GroupQuery == "", hasIn: in.GroupQuery != ""},
		{key: "groupLabels", empty: len(cur.GroupLabels) == 0, hasIn: len(in.GroupLabels) > 0},
		{key: "groupColors", empty: len(cur.GroupColors) == 0, hasIn: len(in.GroupColors) > 0},
		{key: "productByGroup", empty: len(cur.ProductByGroup) == 0, hasIn: len(in.ProductByGroup) > 0},
		{key: "features", empty: len(cur.Features) == 0, hasIn: len(in.Features) > 0},
		{key: "qaDashboardUrl", empty: cur.QaDashboardURL == "", hasIn: in.QaDashboardURL != ""},
		{key: "staleThresholdHours", empty: cur.StaleThresholdHours == 0, hasIn: in.StaleThresholdHours != 0},
		{key: "confluence", empty: cur.Confluence == nil, hasIn: in.Confluence != nil},
	}
	var out []SettingChange
	for _, it := range items {
		if !it.hasIn {
			continue
		}
		if it.empty {
			out = append(out, SettingChange{Key: it.key, Action: SettingAdd})
			continue
		}
		if overwrite {
			out = append(out, SettingChange{Key: it.key, Action: SettingReplace})
		} else {
			out = append(out, SettingChange{Key: it.key, Action: SettingSkip})
		}
	}
	return out
}

// ApplyPlan mutates cfg in place for settings changes and writes views via db.
// Credential fields on cfg are never assigned — only whitelist keys from the plan.
// Caller must Save() the config after a successful settings apply when not dry-run.
func ApplyPlan(cfg *config.Config, db *store.DB, plan Plan) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	for _, sc := range plan.Settings {
		if sc.Action == SettingSkip {
			continue
		}
		if err := applySetting(cfg, sc.Key, plan.Incoming.Settings); err != nil {
			return err
		}
	}
	for _, vc := range plan.Views {
		switch vc.Action {
		case ViewSkip:
			continue
		case ViewAdd:
			id, err := newViewID()
			if err != nil {
				return err
			}
			if err := db.PutSavedView(context.Background(), store.SavedView{
				ID:     id,
				Name:   vc.Name,
				Config: vc.Config,
			}); err != nil {
				return err
			}
		case ViewReplace:
			if vc.ExistingID == "" {
				return fmt.Errorf("view replace %q: missing existing id", vc.Name)
			}
			// Preserve created_at by reading current row if present.
			created := ""
			if views, err := db.SavedViews(context.Background()); err == nil {
				for _, v := range views {
					if v.ID == vc.ExistingID {
						created = v.CreatedAt
						break
					}
				}
			}
			if err := db.PutSavedView(context.Background(), store.SavedView{
				ID:        vc.ExistingID,
				Name:      vc.Name,
				Config:    vc.Config,
				CreatedAt: created,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func applySetting(cfg *config.Config, key string, in TeamSettings) error {
	switch key {
	case "projects":
		cfg.Projects = copyStrings(in.Projects)
	case "fields":
		cfg.Fields = copyFieldSpecs(in.Fields)
	case "bodyFields":
		cfg.BodyFields = copyStrings(in.BodyFields)
	case "members":
		cfg.Members = copyMembers(in.Members)
	case "groupRules":
		cfg.GroupRules = copyGroupRules(in.GroupRules)
	case "groupQuery":
		cfg.GroupQuery = in.GroupQuery
	case "groupLabels":
		cfg.GroupLabels = copyStringMap(in.GroupLabels)
	case "groupColors":
		cfg.GroupColors = copyStringMap(in.GroupColors)
	case "productByGroup":
		cfg.ProductByGroup = copyProductMap(in.ProductByGroup)
	case "features":
		cfg.Features = copyBoolMap(in.Features)
	case "qaDashboardUrl":
		cfg.QaDashboardURL = in.QaDashboardURL
	case "staleThresholdHours":
		cfg.StaleThresholdHours = in.StaleThresholdHours
	case "confluence":
		cfg.Confluence = copyConfluence(in.Confluence)
	default:
		return fmt.Errorf("internal: unknown settings key %q", key)
	}
	return nil
}

func newViewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
