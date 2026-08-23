package jql

import "time"

// Filter is the JSON shape of web/src/lib/view-config.ts ViewFilters.
// Empty slices marshal as [] so a client can apply the object wholesale.
type Filter struct {
	StatusCategory    []string            `json:"status_category"`
	StatusCategoryNot []string            `json:"status_category_not"`
	Status            []string            `json:"status"`
	StatusNot         []string            `json:"status_not"`
	AssigneeEmail     []string            `json:"assignee_email"`
	AssigneeEmailNot  []string            `json:"assignee_email_not"`
	ReporterEmail     []string            `json:"reporter_email"`
	ReporterEmailNot  []string            `json:"reporter_email_not"`
	TeamGroup         []string            `json:"team_group"`
	TeamGroupNot      []string            `json:"team_group_not"`
	Labels            []string            `json:"labels"`
	LabelsNot         []string            `json:"labels_not"`
	Priority          []string            `json:"priority"`
	PriorityNot       []string            `json:"priority_not"`
	Severity          []string            `json:"severity"`
	SeverityNot       []string            `json:"severity_not"`
	IssueType         []string            `json:"issue_type"`
	IssueTypeNot      []string            `json:"issue_type_not"`
	Components        []string            `json:"components"`
	ComponentsNot     []string            `json:"components_not"`
	FixVersions       []string            `json:"fix_versions"`
	FixVersionsNot    []string            `json:"fix_versions_not"`
	QARun             []string            `json:"qa_run"`
	QARunNot          []string            `json:"qa_run_not"`
	QASuite           []string            `json:"qa_suite"`
	QASuiteNot        []string            `json:"qa_suite_not"`
	QAImpact          []string            `json:"qa_impact"`
	QAImpactNot       []string            `json:"qa_impact_not"`
	DeployState       []string            `json:"deploy_state"`
	DeployStateNot    []string            `json:"deploy_state_not"`
	JiraProject       []string            `json:"jira_project"`
	SourceProject     []string            `json:"source_project"`
	JiraProjectNot    []string            `json:"jira_project_not"`
	SourceProjectNot  []string            `json:"source_project_not"`
	Keys              []string            `json:"keys"`
	Parent            []string            `json:"parent"`
	SprintIDs         []string            `json:"sprint_ids"`
	SprintState       []string            `json:"sprint_state"`
	Fields            map[string][]string `json:"fields"`
	Reopened          bool                `json:"reopened"`
	Unassigned        bool                `json:"unassigned"`
	Stale             bool                `json:"stale"`
	CreatedFrom       *string             `json:"created_from"`
	CreatedTo         *string             `json:"created_to"`
	UpdatedFrom       *string             `json:"updated_from"`
	UpdatedTo         *string             `json:"updated_to"`
	DueFrom           *string             `json:"due_from"`
	DueTo             *string             `json:"due_to"`
	ResolvedFrom      *string             `json:"resolved_from"`
	ResolvedTo        *string             `json:"resolved_to"`
	Q                 string              `json:"q"`
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
	ParentKey      string
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
	Duedate        string
	ResolvedAt     string
	SprintID       string
	SprintState    string
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
		StatusCategory:    []string{},
		StatusCategoryNot: []string{},
		Status:            []string{},
		StatusNot:         []string{},
		AssigneeEmail:     []string{},
		AssigneeEmailNot:  []string{},
		ReporterEmail:     []string{},
		ReporterEmailNot:  []string{},
		TeamGroup:         []string{},
		TeamGroupNot:      []string{},
		Labels:            []string{},
		LabelsNot:         []string{},
		Priority:          []string{},
		PriorityNot:       []string{},
		Severity:          []string{},
		SeverityNot:       []string{},
		IssueType:         []string{},
		IssueTypeNot:      []string{},
		Components:        []string{},
		ComponentsNot:     []string{},
		FixVersions:       []string{},
		FixVersionsNot:    []string{},
		QARun:             []string{},
		QARunNot:          []string{},
		QASuite:           []string{},
		QASuiteNot:        []string{},
		QAImpact:          []string{},
		QAImpactNot:       []string{},
		DeployState:       []string{},
		DeployStateNot:    []string{},
		JiraProject:       []string{},
		SourceProject:     []string{},
		JiraProjectNot:    []string{},
		SourceProjectNot:  []string{},
		Keys:              []string{},
		Parent:            []string{},
		SprintIDs:         []string{},
		SprintState:       []string{},
		Fields:            map[string][]string{},
	}
}
