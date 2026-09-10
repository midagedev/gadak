package main

// Comment @mention resolution: turning `@kim` in a comment body into a
// real account id, with the ambiguity and unresolved-mention reporting a
// comment author needs before the write lands. Split from agent.go
// (GDK-1771); the comment verbs that call into this live in
// agent_write.go.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

const maxMentionWords = 3

// resolvedMention is one token the origin accepted, carried for the stderr
// notice that mirrors warnUnresolvedMentions: the author should see who each
// token became, not only what failed to (GDK-894).
type resolvedMention struct {
	token string
	name  string
}

// resolveCommentMentions turns typed `@Name` tokens into the map jira.Doc
// expects: key = the exact substring the user typed (no `@`), value = account
// id. The body is not rewritten — Jira renders the display name from the id.
//
// Hits: exactly one → mention node; two or more → refuse the write; zero →
// leave the token as plain text and name it for the caller to warn about.
func resolveCommentMentions(ctx context.Context, c origin.Writer, body string) (mentions map[string]string, resolved []resolvedMention, unresolved []string, err error) {
	sites := mentionSites(body)
	if len(sites) == 0 {
		return nil, nil, nil, nil
	}
	cache := make(map[string][]jira.User)
	mentions = make(map[string]string)
	seenUnresolved := map[string]bool{}
	seenResolved := map[string]bool{}
	for _, candidates := range sites {
		token, id, users, err := resolveMentionSite(ctx, c, cache, candidates)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(users) >= 2 {
			return nil, nil, nil, ambiguousMention(token, users)
		}
		if id != "" {
			mentions[token] = id
			if !seenResolved[token] {
				seenResolved[token] = true
				name := users[0].DisplayName
				if name == "" {
					name = users[0].ID()
				}
				resolved = append(resolved, resolvedMention{token: token, name: name})
			}
			continue
		}
		if token != "" && !seenUnresolved[token] {
			seenUnresolved[token] = true
			unresolved = append(unresolved, token)
		}
	}
	return mentions, resolved, unresolved, nil
}

// resolveMentionSite walks the candidates shortest-first and stops at the
// first that names exactly one user.
//
// Shortest-first is the whole contract. Longest-first looks safer and is
// wrong against a real origin: Jira's user search is fuzzy, so a query of
// "김현철 GDK-510 멘션" still returns 김현철, the three-word candidate wins,
// and the mention node swallows the two words the author actually wrote
// (measured on a live site 2026-08-21, before this rule existed). Extending
// past a shorter name is only justified when that name failed to identify
// one person — which is what the web UI's autocomplete does too.
func resolveMentionSite(ctx context.Context, c origin.Writer, cache map[string][]jira.User, candidates []string) (token, id string, users []jira.User, err error) {
	var ambiguousToken string
	var ambiguousUsers []jira.User
	for _, cand := range candidates {
		hits, err := lookupMentionUsers(ctx, c, cache, cand)
		if err != nil {
			return "", "", nil, err
		}
		hits = plausibleMentionHits(hits, cand)
		if len(hits) == 1 && hits[0].ID() != "" {
			return cand, hits[0].ID(), hits, nil
		}
		// Two hits mean this name is short of one person; a longer name may
		// still resolve. Keep the shortest ambiguity to report if none does.
		if len(hits) >= 2 && ambiguousToken == "" {
			ambiguousToken, ambiguousUsers = cand, hits
		}
	}
	if ambiguousToken != "" {
		return ambiguousToken, "", ambiguousUsers, nil
	}
	// Nothing matched: name the shortest candidate, which is the token the
	// author typed rather than the words that follow it.
	if len(candidates) > 0 {
		return candidates[0], "", nil, nil
	}
	return "", "", nil, nil
}

// plausibleMentionHits keeps only the hits whose display name or email
// actually contains the typed token, case-folded. The origin's user search is
// fuzzy, and on some sites a token that names nobody returns every user
// instead — measured 2026-08-29 (GDK-894): a plain table-cell word matched
// 18/18 users, none of them containing it, and the comment was refused as
// "ambiguous". Containment turns all-match back into the no-match it is;
// legitimate fuzzy hits (a partial name, an email prefix) contain the token
// and pass.
func plausibleMentionHits(hits []jira.User, token string) []jira.User {
	if len(hits) == 0 || token == "" {
		return hits
	}
	folded := foldForMention(token)
	out := make([]jira.User, 0, len(hits))
	for _, u := range hits {
		if strings.Contains(foldForMention(u.DisplayName), folded) ||
			(u.Email != "" && strings.Contains(foldForMention(u.Email), folded)) {
			out = append(out, u)
		}
	}
	return out
}

// foldForMention case-folds a string for substring matching: every rune maps
// to the smallest rune of its Unicode simple-folding orbit, so Korean and
// other caseless scripts pass through unchanged. stdlib-only on purpose —
// this round adds no third-party import.
func foldForMention(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		low := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < low {
				low = f
			}
		}
		b.WriteRune(low)
	}
	return b.String()
}

func lookupMentionUsers(ctx context.Context, c origin.Writer, cache map[string][]jira.User, q string) ([]jira.User, error) {
	if u, ok := cache[q]; ok {
		return u, nil
	}
	u, err := c.SearchUsers(ctx, q)
	if err != nil {
		return nil, err
	}
	if u == nil {
		u = []jira.User{}
	}
	cache[q] = u
	return u, nil
}

func ambiguousMention(token string, users []jira.User) error {
	names := make([]string, 0, len(users))
	for _, u := range users {
		name := u.DisplayName
		if name == "" {
			name = u.ID()
		}
		names = append(names, name)
	}
	// Two or more hits is the opposite of "no user matching": the refusal has
	// to say the name is over-specified, not absent, or the next attempt is a
	// longer search for someone who was already found twice. When the @word was
	// never meant as a person, the escape is backticks — a code span is not a
	// mention site (GDK-843), and saying so ends the guessing.
	return fmt.Errorf("@%s matches %d users on this origin (%s) — nothing was posted. Type enough of the name that exactly one matches, or wrap a literal @word in backticks",
		token, len(users), strings.Join(names, "; "))
}

func warnUnresolvedMentions(tokens []string) {
	if len(tokens) == 0 {
		return
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = "@" + t
	}
	// The same line must say the write landed (GDK-1544): "did not resolve"
	// alone reads as failure to an agent, which retried or rewrote a body
	// that had posted fine.
	fmt.Fprintf(os.Stderr, "gadak: %s did not resolve to a user on this origin — left as plain text; the comment was saved as typed\n", strings.Join(parts, ", "))
}

// noticeResolvedMentions is the twin of warnUnresolvedMentions for the
// success side: the body that reaches the origin differs from what was typed,
// and that must never happen silently (GDK-894).
func noticeResolvedMentions(ms []resolvedMention) {
	if len(ms) == 0 {
		return
	}
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = "@" + m.token + " -> " + m.name
	}
	fmt.Fprintf(os.Stderr, "gadak: mention: %s\n", strings.Join(parts, ", "))
}

// mentionSites returns one candidate list per `@` that starts a mention
// (string start or immediately after whitespace). Each list is shortest-first,
// at most maxMentionWords exact substrings of the body after `@` — see
// resolveMentionSite for why the order is not the other way round.
//
// Four shapes never become sites at all: an `@` inside markdown code
// (GDK-894) — jira.FindCodeRegions is the same region judgment the
// substitution side uses, so extraction and substitution cannot disagree — an
// `@` whose first word contains `/`, which is a package path or handle, not a
// person, an `@` naming a CSS at-rule (GDK-1125, GDK-1544), which is
// stylesheet data quoted into prose, and an `@` whose first word is written in
// runes no name uses (GDK-976) — a pixel-density "@3×)" is technical data.
// Dropping a site is the safe direction: the token stays plain text and nobody
// is summoned. The reverse — treating code as a person — once nearly turned
// `@xterm/xterm` into a user mention.
func mentionSites(body string) [][]string {
	regions := jira.FindCodeRegions(body)
	var sites [][]string
	for i := 0; i < len(body); {
		r, size := utf8.DecodeRuneInString(body[i:])
		if r == '@' && mentionStartsAt(body, i) && !regions.Cover(i) {
			rest := body[i+size:]
			if !mentionFirstWordHasSlash(rest) && !mentionIsCSSAtRule(rest) {
				if cands := mentionWordCandidates(rest); len(cands) > 0 && mentionPlausibleName(cands[0]) {
					sites = append(sites, cands)
				}
			}
		}
		i += size
	}
	return sites
}

// cssAtRuleNames is the single table of at-rule names an `@`-token is never
// a person for. A comment quoting CSS in prose — "@theme 밖이라…" (GDK-1544),
// a bare media query (GDK-1125) — used to ask the origin's user search for
// every one of them and come back as a stderr warning an agent could read as
// a failed write. One table, one lookup (mentionIsCSSAtRule): extending the
// class is a line here, nowhere else. CSS itself is case-insensitive, so the
// lookup folds.
var cssAtRuleNames = []string{
	"charset", "container", "font-face", "import", "keyframes", "layer",
	"media", "namespace", "page", "property", "supports", "theme",
}

// cssAtRules is cssAtRuleNames keyed in foldForMention form — smallest rune
// of each fold orbit, uppercase for Latin — the same fold the query side
// applies, so the lookup is a plain map hit.
var cssAtRules = func() map[string]bool {
	m := make(map[string]bool, len(cssAtRuleNames))
	for _, n := range cssAtRuleNames {
		m[foldForMention(n)] = true
	}
	return m
}()

// mentionIsCSSAtRule reports whether rest — the text glued after an `@` —
// starts with one of the known CSS at-rule names. The identifier run is
// letters, digits, and hyphen ("font-face", "keyframes"), so a glued paren or
// brace still identifies the rule: `@media(…)` is media. A real person named
// after an at-rule stays plain text — the same trade every drop class makes.
func mentionIsCSSAtRule(rest string) bool {
	var name []rune
	for _, r := range rest {
		if r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		name = append(name, r)
	}
	return len(name) > 0 && cssAtRules[foldForMention(string(name))]
}

// mentionPlausibleName reports whether every rune of cand is in the alphabet
// names and addresses are written in: unicode letters and digits, plus the
// punctuation real ones carry — hyphens, apostrophes, the period in
// "St. Clair", the underscore in a handle, and the @ and + of an email
// address (GDK-510's email provision resolves these, so the filter must pass
// them). Technical notations glued after an `@` fail it: the × and ) of
// "@3×)" are in no name alphabet (GDK-976). It is a rune filter and not a
// letters-of-the-English-alphabet check, because a Korean display name is a
// first-class mention. The check runs on the first candidate — a site's
// shortest form, which every longer candidate extends — and an ASCII "@2x"
// passes it: the issue's own out, since a handle that could be real deserves
// the search, and the search leaving it as typed is the safe result.
func mentionPlausibleName(cand string) bool {
	if cand == "" {
		return false
	}
	for _, r := range cand {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
		case r == '-' || r == '_' || r == '.' || r == '\'' || r == '@' || r == '+':
		default:
			return false
		}
	}
	return true
}

// mentionFirstWordHasSlash reports whether the first word after an `@`
// contains `/` — `@xterm/xterm`, `@types/node`, a social handle. Display
// names with a slash are vanishingly rare, and one that slips through merely
// stays plain text.
func mentionFirstWordHasSlash(rest string) bool {
	for _, r := range rest {
		if r == '/' {
			return true
		}
		if unicode.IsSpace(r) {
			return false
		}
	}
	return false
}

func mentionStartsAt(body string, i int) bool {
	if i == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(body[:i])
	return unicode.IsSpace(prev)
}

func mentionWordCandidates(rest string) []string {
	ends := wordEndOffsets(rest, maxMentionWords)
	if len(ends) == 0 {
		return nil
	}
	out := make([]string, 0, len(ends))
	for n := 1; n <= len(ends); n++ {
		tok := strings.TrimRight(rest[:ends[n-1]], ",.;:!?")
		if tok == "" {
			continue
		}
		out = append(out, tok)
	}
	return out
}

func wordEndOffsets(s string, n int) []int {
	var ends []int
	inWord := false
	last := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '@' && !inWord {
			// A later `@Name` is its own site, not a word of this one.
			// Consuming it would spend the 3-query budget on "Dana @Kim"
			// and can make two clear mentions look ambiguous.
			return ends
		}
		if unicode.IsSpace(r) {
			if inWord {
				ends = append(ends, i)
				inWord = false
				if len(ends) == n {
					return ends
				}
			}
		} else {
			inWord = true
			last = i + size
		}
		i += size
	}
	if inWord && len(ends) < n {
		ends = append(ends, last)
	}
	return ends
}
