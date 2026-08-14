package jql

import "time"

// Filter is the JSON shape of web/src/lib/view-config.ts ViewFilters.
// Empty slices marshal as [] so a client can apply the object wholesale.
type Filter struct {
	StatusCategory []string            `json:"status_category"`
	Status         []string            `json:"status"`
	AssigneeEmail  []string            `json:"assignee_email"`
	ReporterEmail  []string            `json:"reporter_email"`
	TeamGroup      []string            `json:"team_group"`
	Labels         []string            `json:"labels"`
	Priority       []string            `json:"priority"`
	Severity       []string            `json:"severity"`
	IssueType      []string            `json:"issue_type"`
	Components     []string            `json:"components"`
	FixVersions    []string            `json:"fix_versions"`
	QARun          []string            `json:"qa_run"`
	QASuite        []string            `json:"qa_suite"`
	QAImpact       []string            `json:"qa_impact"`
	DeployState    []string            `json:"deploy_state"`
	JiraProject    []string            `json:"jira_project"`
	SourceProject  []string            `json:"source_project"`
	Keys           []string            `json:"keys"`
	Fields         map[string][]string `json:"fields"`
	Reopened       bool                `json:"reopened"`
	Unassigned     bool                `json:"unassigned"`
	Stale          bool                `json:"stale"`
	CreatedFrom    *string             `json:"created_from"`
	CreatedTo      *string             `json:"created_to"`
	UpdatedFrom    *string             `json:"updated_from"`
	UpdatedTo      *string             `json:"updated_to"`
	Q              string              `json:"q"`
}

// Display is the ORDER BY / grouping fragment. Empty sort/dir means
// "leave the view as-is" on parse; Hash omits gadak defaults.
type Display struct {
	Sort    string `json:"sort,omitempty"`     // updated | created | priority
	Dir     string `json:"dir,omitempty"`      // asc | desc
	GroupBy string `json:"group_by,omitempty"` // status_category is the default
}

// Result is one parse (or emit) outcome. Error is a stable code; Message is
// human. Unsupported clauses were understood and refused; they are never
// silently dropped.
type Result struct {
	Input       string   `json:"input,omitempty"`
	JQL         string   `json:"jql"`
	Filters     Filter   `json:"filters"`
	Display     Display  `json:"display"`
	Applied     []string `json:"applied"`
	Unsupported []string `json:"unsupported"`
	Omitted     []string `json:"omitted,omitempty"`
	Error       string   `json:"error,omitempty"`
	Message     string   `json:"message,omitempty"`
}

// Error codes on Result.Error.
const (
	ErrEmpty       = "empty"
	ErrNotJQL      = "not_jql"
	ErrFilterID    = "filter_id"
	ErrParse       = "parse"
	ErrTooManyKeys = "too_many_keys"
)

// MaxKeys is the compile and --keys ceiling. Above this, Parse and the CLI
// return a loud error that includes the count.
const MaxKeys = 500

// Opts control parse-time evaluation (dates, currentUser).
type Opts struct {
	Now       time.Time
	Email     string // currentUser() fallback when AccountID is empty
	AccountID string // currentUser() body when set
}

// Identity is the configured Jira user ResolvePeople substitutes for currentUser().
type Identity struct {
	Email     string
	AccountID string
}

// Issue is the source-neutral row Match compares to a Filter. Callers map
// their own lite type onto this; the package does not import store.
type Issue struct {
	Key            string
	Project        string
	Status         string
	StatusCategory string
	Type           string
	Priority       string
	Assignee       string
	AssigneeEmail  string
	AssigneeID     string
	Reporter       string
	ReporterEmail  string
	ReporterID     string
	Labels         []string
	Components     []string
	FixVersions    []string
	CreatedAt      string
	UpdatedAt      string
}

// Person is one identity ResolvePeople can match a JQL assignee/reporter
// value against (email, display name, account id).
type Person struct {
	Email       string
	Name        string
	DisplayName string
	AccountID   string
}

// EmptyFilter returns a Filter whose slices are non-nil, ready to marshal.
func EmptyFilter() Filter {
	return Filter{
		StatusCategory: []string{},
		Status:         []string{},
		AssigneeEmail:  []string{},
		ReporterEmail:  []string{},
		TeamGroup:      []string{},
		Labels:         []string{},
		Priority:       []string{},
		Severity:       []string{},
		IssueType:      []string{},
		Components:     []string{},
		FixVersions:    []string{},
		QARun:          []string{},
		QASuite:        []string{},
		QAImpact:       []string{},
		DeployState:    []string{},
		JiraProject:    []string{},
		SourceProject:  []string{},
		Keys:           []string{},
		Fields:         map[string][]string{},
	}
}

func (f Filter) empty() bool {
	if len(f.StatusCategory) > 0 || len(f.Status) > 0 ||
		len(f.AssigneeEmail) > 0 || len(f.ReporterEmail) > 0 ||
		len(f.Labels) > 0 || len(f.Priority) > 0 || len(f.IssueType) > 0 ||
		len(f.Components) > 0 || len(f.FixVersions) > 0 ||
		len(f.JiraProject) > 0 || len(f.Keys) > 0 || f.Unassigned || f.Reopened || f.Stale ||
		f.CreatedFrom != nil || f.CreatedTo != nil ||
		f.UpdatedFrom != nil || f.UpdatedTo != nil || f.Q != "" {
		return false
	}
	return true
}
