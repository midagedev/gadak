package main

// Symbolic issue references across the two seed halves (GDK-45).
//
// A dataset pair (issues + docs) cannot hardcode issue keys: the Jira site
// numbers them at creation time, so the docs half can only know a key after
// the issues half ran. The refmap is the hand-off: issues carry stable local
// symbols (`"ref": "auth-sso-timeout"`), the --data run writes
// symbol → real key, and the --docs run substitutes `{{ref:symbol}}` in
// page bodies and comments before anything is created. An unresolved or
// malformed token is a hard error — a doc that ships with `{{ref:…}}` in
// its body is a doc that was never resolved, not a doc that is "mostly
// done".

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// refToken matches a symbolic reference in a docs body. The name charset is
// deliberately narrow (kebab/case identifiers) so a leftover token can never
// be mistaken for prose.
var refToken = regexp.MustCompile(`\{\{ref:([A-Za-z0-9._-]+)\}\}`)

// refKeyMap pairs every authored ref in dataset order with the key its issue
// received. order and keys are parallel slices (seedFromData builds both);
// a duplicate ref is an authoring error, not a warning — two issues claiming
// one symbol would resolve half the doc links to the wrong issue. A length
// mismatch means creation was partial and the pairing is untrustworthy: a
// ref mapped to the wrong issue's key is worse than no refmap at all.
func refKeyMap(order []SeedIssue, keys []string) (map[string]string, error) {
	if len(order) != len(keys) {
		return nil, fmt.Errorf("created %d of %d planned issues — index pairing is unreliable, not writing a refmap", len(keys), len(order))
	}
	out := make(map[string]string, len(order))
	for i := range order {
		r := order[i].Ref
		if r == "" {
			continue
		}
		if prev, dup := out[r]; dup {
			return nil, fmt.Errorf("duplicate ref %q (issues %s and %s)", r, prev, keys[i])
		}
		out[r] = keys[i]
	}
	return out, nil
}

// writeRefMap persists symbol → key. json.Marshal sorts map keys, so two
// runs over the same site diff cleanly.
func writeRefMap(path string, m map[string]string) error {
	if len(m) == 0 {
		return fmt.Errorf("no issue in the dataset carries a \"ref\" — nothing to write to %s", path)
	}
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0o644)
}

// loadRefMap reads a refmap written by a previous --data run.
func loadRefMap(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// resolveDocsRefs substitutes every {{ref:…}} token in page bodies and page
// comments in place, returning how many tokens resolved. refs may be nil
// (no --refmap): that only stays legal while the dataset carries no tokens
// at all.
//
// Three hard errors, each before any network call: a token with no map
// loaded, a token the map does not know, and a leftover "{{ref:" after
// substitution — the malformed-token catch (unclosed, spaces, unsupported
// charset), which the regexp alone would silently leave in the body.
func resolveDocsRefs(data *DocsDataset, refs map[string]string) (int, error) {
	resolved := 0
	subs := func(where, title, body string) (string, error) {
		var missing []string
		out := refToken.ReplaceAllStringFunc(body, func(tok string) string {
			m := refToken.FindStringSubmatch(tok)
			if m == nil {
				return tok
			}
			key, ok := refs[m[1]]
			if !ok {
				missing = append(missing, m[1])
				return tok
			}
			resolved++
			return key
		})
		// The first unresolved name is the error; the others would say the same
		// thing (staticcheck SA4004 named the range-that-returns shape, CI run
		// 34499901502).
		if len(missing) > 0 {
			name := missing[0]
			if refs == nil {
				return "", fmt.Errorf("%s %q uses {{ref:%s}} but no --refmap was given — run the --data half with --refmap first", where, title, name)
			}
			return "", fmt.Errorf("%s %q references %q, which the refmap does not carry", where, title, name)
		}
		return out, nil
	}

	for i := range data.Pages {
		p := &data.Pages[i]
		body, err := subs("page", p.Title, p.BodyStorage)
		if err != nil {
			return 0, err
		}
		p.BodyStorage = body
		for j := range p.Comments {
			body, err := subs("comment on page", p.Title, p.Comments[j].BodyStorage)
			if err != nil {
				return 0, err
			}
			p.Comments[j].BodyStorage = body
		}
	}

	// Leftover scan: anything the regexp could not consume is malformed.
	for i := range data.Pages {
		p := &data.Pages[i]
		bodies := append([]string{p.BodyStorage}, commentBodies(p)...)
		for _, body := range bodies {
			if left := firstUnresolved(body); left != "" {
				return 0, fmt.Errorf("page %q carries an unresolved token %q — not a valid {{ref:name}}", p.Title, left)
			}
		}
	}
	return resolved, nil
}

// firstUnresolved returns the first "{{ref:"-shaped leftover in body, or "".
func firstUnresolved(body string) string {
	i := strings.Index(body, "{{ref:")
	if i < 0 {
		return ""
	}
	end := strings.IndexByte(body[i:], '}')
	if end < 0 {
		end = min(len(body)-i, 40)
		return body[i : i+end]
	}
	return body[i : i+end+1]
}

func commentBodies(p *DocsPage) []string {
	out := make([]string, 0, len(p.Comments))
	for _, c := range p.Comments {
		out = append(out, c.BodyStorage)
	}
	return out
}
