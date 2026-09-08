package origin

import (
	"encoding/json"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
)

// BodyDialect is how this workspace's origin holds an issue body (GDK-1637):
// wiki markup as a plain string on a Jira Server origin, markdown-shaped text
// everywhere else — Cloud Jira, Linear, and the built-in tracker. The dialect
// comes from the origin type, never from sniffing the string; there is no
// fallback from one to the other.
func BodyDialect(cfg *config.Config) adf.Dialect {
	if cfg != nil && cfg.OriginType() == config.OriginJiraServer {
		return adf.DialectWiki
	}
	return adf.DialectMarkdown
}

// BodyValue is the value this origin's body field takes for the text a person
// typed: doc — the markdown conversion (jira.Doc, jira.DocWithMedia, or
// adf.FromMarkdownWith) — on a markdown origin; the raw string itself on a
// Jira Server origin, whose body field is wiki markup carried verbatim, never
// converted in either direction (GDK-1637). The Server branch does not
// consult doc, so a caller may pass the conversion unconditionally.
func BodyValue(cfg *config.Config, text string, doc json.RawMessage) json.RawMessage {
	if BodyDialect(cfg) == adf.DialectWiki {
		b, _ := json.Marshal(text) // a string cannot fail to marshal
		return b
	}
	return doc
}

// RefuseBodyPlaceholders is the gate every text→body write without a body to
// substitute from runs (create, comment): a placeholder stands for a node of
// the body it was read from, and this write has no such body. A Jira Server
// origin never has: its bodies are verbatim strings with nothing behind a
// marker, so the refusal does not apply (GDK-1637).
func RefuseBodyPlaceholders(cfg *config.Config, text string) error {
	if BodyDialect(cfg) == adf.DialectWiki {
		return nil
	}
	return adf.RefusePlaceholders(text)
}
