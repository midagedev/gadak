package jira

import (
	"encoding/json"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// The payload shapes below are only the parts sync maps. Everything else stays
// in Issue.Raw, which the store keeps as an escape hatch.

type NamedID struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
	// The two below are only ever filled by the user search the assignee picker
	// calls; the mirror stores neither.
	AvatarURLs map[string]string `json:"avatarUrls"`
	Active     bool              `json:"active"`
}

// Avatar is the 48px avatar, or empty when Jira sent none.
func (u User) Avatar() string { return u.AvatarURLs["48x48"] }

// Status carries the category because every piece of logic keys on it: names
// come back in the account's display language (contracts/sync.md, "Localization
// hazard").
type Status struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StatusCategory struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

// CommentVisibility is Jira's comment restriction. Type is "role" or
// "group"; Value is the role or group name. Unrestricted comments omit the
// JSON key (nil here). gadak mirrors the fields; it does not compute who
// may read the comment.
type CommentVisibility struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Comment struct {
	ID         string             `json:"id"`
	Author     User               `json:"author"`
	Body       json.RawMessage    `json:"body"`
	Created    string             `json:"created"`
	Updated    string             `json:"updated"`
	Visibility *CommentVisibility `json:"visibility"`
	JsdPublic  *bool              `json:"jsdPublic"` // nil = key absent; false ≠ absent
}

type CommentPage struct {
	Comments   []Comment `json:"comments"`
	Total      int       `json:"total"`
	MaxResults int       `json:"maxResults"`
	StartAt    int       `json:"startAt"`
}

type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Author   User   `json:"author"`
	Created  string `json:"created"`
}

type IssueLink struct {
	Type struct {
		Name string `json:"name"`
	} `json:"type"`
	InwardIssue *struct {
		Key string `json:"key"`
	} `json:"inwardIssue"`
	OutwardIssue *struct {
		Key string `json:"key"`
	} `json:"outwardIssue"`
}

// HistoryItem's FieldID is the stable identifier ("status", "assignee"); Field
// is the display name and is localized.
type HistoryItem struct {
	Field      string `json:"field"`
	FieldID    string `json:"fieldId"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

type History struct {
	ID      string        `json:"id"`
	Created string        `json:"created"`
	Author  User          `json:"author"`
	Items   []HistoryItem `json:"items"`
}

// Changelog as returned inline by expand=changelog. Total above len(Histories)
// means it was truncated and the dedicated endpoint has to be paged.
type Changelog struct {
	Total      int       `json:"total"`
	MaxResults int       `json:"maxResults"`
	Histories  []History `json:"histories"`
}

type Fields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"`
	Environment json.RawMessage `json:"environment"`
	IssueType   NamedID         `json:"issuetype"`
	Status      Status          `json:"status"`
	Priority    *NamedID        `json:"priority"`
	Assignee    *User           `json:"assignee"`
	Reporter    *User           `json:"reporter"`
	Creator     *User           `json:"creator"`
	Project     struct {
		Key string `json:"key"`
	} `json:"project"`
	Parent *struct {
		Key string `json:"key"`
	} `json:"parent"`
	Labels      []string     `json:"labels"`
	Components  []NamedID    `json:"components"`
	FixVersions []NamedID    `json:"fixVersions"`
	Versions    []NamedID    `json:"versions"` // affects versions
	Duedate     string       `json:"duedate"`
	Resolution  *NamedID     `json:"resolution"`
	Created     string       `json:"created"`
	Updated     string       `json:"updated"`
	Comment     CommentPage  `json:"comment"`
	Attachment  []Attachment `json:"attachment"`
	IssueLinks  []IssueLink  `json:"issuelinks"`
}

// Issue keeps the fields object three ways: typed for the mapping, verbatim per
// field id so configured custom fields need no code, and whole as Raw.
type Issue struct {
	ID        string
	Key       string
	Fields    Fields
	Extra     map[string]json.RawMessage
	Raw       json.RawMessage
	Changelog *Changelog
}

func (i *Issue) UnmarshalJSON(b []byte) error {
	var shell struct {
		ID        string          `json:"id"`
		Key       string          `json:"key"`
		Fields    json.RawMessage `json:"fields"`
		Changelog *Changelog      `json:"changelog"`
	}
	if err := json.Unmarshal(b, &shell); err != nil {
		return err
	}
	i.ID, i.Key, i.Changelog = shell.ID, shell.Key, shell.Changelog
	i.Raw = append(json.RawMessage(nil), b...)
	if len(shell.Fields) == 0 {
		return nil
	}
	if err := json.Unmarshal(shell.Fields, &i.Fields); err != nil {
		return err
	}
	return json.Unmarshal(shell.Fields, &i.Extra)
}

// KnownCategory maps a Jira statusCategory key or a gadak token onto the
// three values data-model.md documents. Unlike Category, unknown keys are
// not folded to "new": write resolvers refuse those so a damaged payload
// cannot move an issue (see transitionCategory).
func KnownCategory(key string) (string, bool) {
	switch key {
	case "done":
		return "done", true
	case "indeterminate", "inprogress":
		return "inprogress", true
	case "new":
		return "new", true
	default:
		return "", false
	}
}

// Category maps Jira's statusCategory key onto the three values data-model.md
// documents. An unknown key becomes "new", which can only ever miss a reopen,
// never invent one.
func Category(key string) string {
	if cat, ok := KnownCategory(key); ok {
		return cat
	}
	return "new"
}

// CategoryKey is the reverse of Category: a gadak token (or a Jira key
// Category would accept) onto Jira's REST statusCategory key. inprogress
// becomes indeterminate, the key Cloud actually stores.
func CategoryKey(token string) string {
	switch Category(token) {
	case "inprogress":
		return "indeterminate"
	case "done":
		return "done"
	default:
		return "new"
	}
}

// Layout is how Jira stamps timestamps: ISO-8601 with a numeric offset and no
// colon in it, which is why time.RFC3339 does not parse them.
const Layout = "2006-01-02T15:04:05.000-0700"

// ISOTime normalizes a Jira timestamp to the ISO-8601 UTC form every stored
// column uses (data-model.md, "Conventions"), so string comparison sorts
// chronologically. An unparseable value passes through untouched.
func ISOTime(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(Layout, s)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return s
		}
	}
	return t.UTC().Format(config.ISOMilli)
}
