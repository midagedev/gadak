package jira

import (
	"encoding/json"

	"github.com/midagedev/gadak/internal/adf"
)

// PlainText flattens an ADF document to plain text: it is what FTS indexes and
// what makes a repro-steps custom field searchable. A field that holds a bare
// string (the older wiki-markup shape) passes through unchanged.
func PlainText(raw json.RawMessage) string { return adf.PlainText(raw) }
