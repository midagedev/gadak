// Package origin is the single owner of "this workspace's Jira client".
//
// Production code constructs a *jira.Client only through Client (the
// workspace's origin) or Connected (a candidate site/email/token, used
// when verifying a credential the user just typed). jira.New stays
// exported for tests that stand up httptest servers; a gate test fails
// if production files grow a new direct call.
package origin
