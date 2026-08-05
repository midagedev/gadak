package teamconfig

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/secretscan"
	"github.com/midagedev/scry/internal/store"
)

// CurrentFormat is the only scry_team_config version this build understands.
const CurrentFormat = 1

// Document is the on-disk team share file.
type Document struct {
	Version    int          `json:"scry_team_config"`
	ExportedAt string       `json:"exported_at"`
	Settings   TeamSettings `json:"settings"`
	Views      []ExportView `json:"views"`
}

// TeamSettings holds only the whitelist of Config fields that may be shared.
// JSON tags match config.Config so a human can diff against ~/.scry/config.json.
type TeamSettings struct {
	Projects            []string                  `json:"projects,omitempty"`
	Fields              []config.FieldSpec        `json:"fields,omitempty"`
	FieldMap            map[string]string         `json:"fieldMap,omitempty"`
	BodyFields          []string                  `json:"bodyFields,omitempty"`
	EditableFields      map[string]string         `json:"editableFields,omitempty"`
	Members             []config.Member           `json:"members,omitempty"`
	GroupRules          []config.GroupRule        `json:"groupRules,omitempty"`
	GroupLabels         map[string]string         `json:"groupLabels,omitempty"`
	GroupColors         map[string]string         `json:"groupColors,omitempty"`
	ProductByGroup      map[string]config.Product `json:"productByGroup,omitempty"`
	Features            map[string]bool           `json:"features,omitempty"`
	QaDashboardURL      string                    `json:"qaDashboardUrl,omitempty"`
	StaleThresholdHours int                       `json:"staleThresholdHours,omitempty"`
}

// ExportView is a saved view without machine-local id/timestamps.
type ExportView struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

// ExportOptions controls optional inclusions.
type ExportOptions struct {
	// WithMembers includes Members (emails). Default false.
	WithMembers bool
	// Now overrides the export timestamp (tests). Zero means time.Now().UTC().
	Now time.Time
}

// BuildDocument copies whitelist settings and views into a Document.
// It never copies Site/Email/Token or other never-export fields.
func BuildDocument(cfg *config.Config, views []store.SavedView, opts ExportOptions) Document {
	if cfg == nil {
		cfg = &config.Config{}
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s := TeamSettings{
		Projects:            copyStrings(cfg.Projects),
		Fields:              copyFieldSpecs(cfg.Fields),
		FieldMap:            copyStringMap(cfg.FieldMap),
		BodyFields:          copyStrings(cfg.BodyFields),
		EditableFields:      copyStringMap(cfg.EditableFields),
		GroupRules:          copyGroupRules(cfg.GroupRules),
		GroupLabels:         copyStringMap(cfg.GroupLabels),
		GroupColors:         copyStringMap(cfg.GroupColors),
		ProductByGroup:      copyProductMap(cfg.ProductByGroup),
		Features:            copyBoolMap(cfg.Features),
		QaDashboardURL:      cfg.QaDashboardURL,
		StaleThresholdHours: cfg.StaleThresholdHours,
	}
	if opts.WithMembers {
		s.Members = copyMembers(cfg.Members)
	}
	outViews := make([]ExportView, 0, len(views))
	for _, v := range views {
		cfgRaw := v.Config
		if len(cfgRaw) == 0 {
			cfgRaw = json.RawMessage("{}")
		} else {
			// Re-marshal to normalize spacing for stable git diffs.
			var any any
			if err := json.Unmarshal(cfgRaw, &any); err == nil {
				if b, err := json.Marshal(any); err == nil {
					cfgRaw = b
				}
			}
		}
		outViews = append(outViews, ExportView{Name: v.Name, Config: cfgRaw})
	}
	// Stable view order for readable git diffs.
	sort.SliceStable(outViews, func(i, j int) bool {
		return outViews[i].Name < outViews[j].Name
	})
	return Document{
		Version:    CurrentFormat,
		ExportedAt: now.UTC().Format(time.RFC3339),
		Settings:   s,
		Views:      outViews,
	}
}

// MarshalDocument encodes with 2-space indent for human/git-friendly files.
// Before returning, it scans the bytes for credential-shaped strings and
// refuses to emit them (defense in depth above the field whitelist).
func MarshalDocument(doc Document) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if err := ScanBytesForCredentials(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ScanBytesForCredentials returns an error if raw contains a known secret shape
// (Atlassian API token, Bearer/Basic, GitHub/Slack tokens, PEM private key).
// Email addresses are intentionally not scanned — --with-members may include them.
// Reuses internal/secretscan so this and snapshot generation cannot drift apart.
func ScanBytesForCredentials(raw []byte) error {
	if name := secretscan.Match(string(raw)); name != "" {
		return fmt.Errorf("refusing to write team config: credential-shaped string detected (pattern=%s)", name)
	}
	return nil
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyGroupRules(in []config.GroupRule) []config.GroupRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.GroupRule, len(in))
	for i, r := range in {
		out[i] = config.GroupRule{
			Group:      r.Group,
			Projects:   copyStrings(r.Projects),
			Labels:     copyStrings(r.Labels),
			Components: copyStrings(r.Components),
		}
	}
	return out
}

func copyProductMap(in map[string]config.Product) map[string]config.Product {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]config.Product, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyMembers(in []config.Member) []config.Member {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.Member, len(in))
	copy(out, in)
	return out
}

func copyFieldSpecs(in []config.FieldSpec) []config.FieldSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.FieldSpec, len(in))
	for i, s := range in {
		out[i] = s
		if s.IDs != nil {
			out[i].IDs = append([]string(nil), s.IDs...)
		}
	}
	return out
}
