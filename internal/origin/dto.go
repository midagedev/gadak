package origin

import "github.com/midagedev/gadak/internal/jira"

// Writer DTOs (GDK-665). origin.Writer speaks these names so a new origin
// does not have to import internal/jira to implement the write surface.
//
// They alias the Jira HTTP payload structs rather than restating a subset:
// a distinct type would force internal/server, cmd/gadak stubs,
// internal/transition, and internal/jirafields to name origin types — this
// round's whitelist forbids that, and the HTTP JSON those packages emit
// must stay unchanged. Callers of each field are listed in the round report.
//
// jira.Client unmarshals the HTTP payloads and its methods are the Writer
// methods as-is (GDK-689 removed the identity-conversion frame that used to
// sit between them — an alias needs no adapter).

type (
	Transition          = jira.Transition
	User                = jira.User
	CreateMetaProject   = jira.CreateMetaProject
	CreateMetaIssueType = jira.CreateMetaIssueType
	FieldMeta           = jira.FieldMeta
	NamedID             = jira.NamedID
	Comment             = jira.Comment
	CommentVisibility   = jira.CommentVisibility
	Attachment          = jira.Attachment
	Version             = jira.Version
	IssueLink           = jira.IssueLink
	IssueLinkType       = jira.IssueLinkType
	RemoteLink          = jira.RemoteLink
	CreateFieldMeta     = jira.CreateFieldMeta
)
