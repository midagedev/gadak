// Package jql translates a documented JQL subset to and from gadak's
// in-memory filter (the same shape the web UI serializes into the URL).
//
// It is not a JQL engine. Clauses the subset cannot express — WAS, CHANGED,
// sprint, cross-field OR, !=, saved filter ids — are listed on the result
// and never applied. Silence is the failure mode this package exists to
// prevent.
//
// The store stays source-neutral: this package does not import it, does
// not write SQL, and does not talk to Jira.
package jql
