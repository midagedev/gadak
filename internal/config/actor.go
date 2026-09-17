package config

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// Actor kinds (GDK-1973). "" is the pre-1973 block — an agent — kept
// byte-for-byte; "agent" says it out loud; "person" makes the block a
// person's identity, whose slug may be derived from the display name.
const (
	ActorKindAgent  = "agent"
	ActorKindPerson = "person"
)

// ActorConfig is the workspace-default acting identity for writes to an
// issuetap origin (builtIn or paired, GDK-586): the slug becomes the
// origin accountId verbatim and names an account there; Name is an
// optional display name. Kind says which sort of account it names — an
// agent (the default, and what env/auto detection always resolves) or a
// person (`gadak me set` writes person blocks; issuetap provisions those
// as human accounts). Nil (or empty slug) means unset. Per-machine
// identity — never team-exported.
type ActorConfig struct {
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`

	// Trailer turns off the Jira/Linear attribution line (nil = true, the
	// default): with an actor resolved and the origin unable to record one,
	// agent-authored comments and new issues carry one trailing
	// "— via gadak · <actor>" line (origin.ActorTrailer). The switch is also
	// the switch for any ledger derived from the line — a trailer-less
	// ledger would be a fact that exists nowhere in the origin.
	Trailer *bool `json:"trailer,omitempty"`
}

// ActorTrailerEnabled is the actor.trailer switch: true unless explicitly
// false. Nil-safe — a config that failed to load keeps the default.
func (c *Config) ActorTrailerEnabled() bool {
	return c == nil || c.Actor == nil || c.Actor.Trailer == nil || *c.Actor.Trailer
}

// Actor sources, in ladder order. status reports which rung answered so an
// agent can see that its environment was (or was not) recognized.
const (
	ActorSourceEnv    = "env"    // GADAK_ACTOR
	ActorSourceConfig = "config" // config.json actor
	ActorSourceAuto   = "auto"   // Claude Code detection
)

// ResolvedActor is the ladder's answer: Slug is the X-Issuetap-Actor value,
// Name the optional X-Issuetap-Actor-Name, Source which rung produced it,
// Kind agent or person (never empty once resolved — env, auto-detection,
// and a kindless config block are agents; only a config block that says
// person resolves as one, GDK-1973).
type ResolvedActor struct {
	Slug   string `json:"slug"`
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
}

// maxActorSlugLen mirrors issuetap's X-Issuetap-Actor cap (gadak GDK-588):
// a longer slug is a 400 on every write. Mirrored rather than imported —
// issuetap's api package is internal to that module. The config-set path
// rejects early so the mistake surfaces once, not per request. The env
// rung does not pre-validate: a tool-provided slug deserves the origin's
// own error, not a silently trimmed identity.
const maxActorSlugLen = 128

// ResolveActor is the single owner of "who does this process write as"
// (GDK-586). Ladder, first match wins:
//
//  1. env GADAK_ACTOR — "slug" or "slug|display name"
//  2. the config.json actor block (the workspace default)
//  3. Claude Code auto-detection: CLAUDECODE=1 derives "claude:<first 8 of
//     CLAUDE_CODE_SESSION_ID>" (or bare "claude" with no session id),
//     display name "Claude Code"
//
// Nothing set means no actor: origins see the identity they always did
// (the in-process user / seed account) and no header is sent.
//
// AI_AGENT is deliberately not a slug source: as measured it carries the
// harness version ("claude-code_2-1-239_agent"), so a slug built from it
// would mint a new agent identity per upgrade — the opposite of the
// stable-slug contract.
// ParseActorShorthand splits the one-line actor form shared by
// GADAK_ACTOR and `gadak config set actor`: "slug" or "slug|display name".
// An empty slug means unset.
func ParseActorShorthand(v string) (slug, name string) {
	slug, name, _ = strings.Cut(strings.TrimSpace(v), "|")
	return strings.TrimSpace(slug), strings.TrimSpace(name)
}

func ResolveActor(cfg *Config) (ResolvedActor, bool) {
	if v := strings.TrimSpace(os.Getenv("GADAK_ACTOR")); v != "" {
		if slug, name := ParseActorShorthand(v); slug != "" {
			return ResolvedActor{Slug: slug, Name: name, Kind: ActorKindAgent, Source: ActorSourceEnv}, true
		}
		// An empty slug ("|name") is treated as unset, the same as Env()
		// treats an empty GADAK_* value: fall through, do not fail.
	}
	if cfg != nil && cfg.Actor != nil {
		if slug := strings.TrimSpace(cfg.Actor.Slug); slug != "" {
			kind := ActorKindAgent
			if cfg.Actor.Kind == ActorKindPerson {
				kind = ActorKindPerson
			}
			return ResolvedActor{
				Slug:   slug,
				Name:   strings.TrimSpace(cfg.Actor.Name),
				Kind:   kind,
				Source: ActorSourceConfig,
			}, true
		}
	}
	if os.Getenv("CLAUDECODE") == "1" {
		slug := "claude"
		if sid := strings.TrimSpace(os.Getenv("CLAUDE_CODE_SESSION_ID")); sid != "" {
			// Session ids are UUIDs (ASCII); 8 characters is stable within
			// a session and short enough to read in a UI.
			if len(sid) > 8 {
				sid = sid[:8]
			}
			slug = "claude:" + sid
		}
		return ResolvedActor{Slug: slug, Name: "Claude Code", Kind: ActorKindAgent, Source: ActorSourceAuto}, true
	}
	return ResolvedActor{}, false
}

// actorOrZero is the current actor block by value, so the actor.trailer
// leaf setter starts from what is stored (a trailer flip must not drop a
// configured slug, and a slug-only block must not fabricate a name).
func (c *Config) actorOrZero() ActorConfig {
	if c == nil || c.Actor == nil {
		return ActorConfig{}
	}
	return *c.Actor
}

// ValidateActor is the `gadak config set actor` rule: empty slug clears the
// block; a non-empty slug must be a stable identity, not a display name —
// no whitespace, within issuetap's cap. kind is "", "agent", or "person"
// (GDK-1973): a person block with no slug derives one from the name via
// PersonSlug — the one derivation point — and a person with no name is
// refused, because there is nothing to derive from and nothing to display.
func ValidateActor(slug, name, kind string) (*ActorConfig, error) {
	slug = strings.TrimSpace(slug)
	name = strings.TrimSpace(name)
	if kind != "" && kind != ActorKindAgent && kind != ActorKindPerson {
		return nil, fmt.Errorf("actor.kind must be %q or %q (got %q)", ActorKindAgent, ActorKindPerson, kind)
	}
	if slug == "" {
		if kind == ActorKindPerson {
			if name == "" {
				return nil, fmt.Errorf("a person needs a display name — `gadak me set \"Your Name\"`")
			}
			return &ActorConfig{Slug: PersonSlug(name), Name: name, Kind: kind}, nil
		}
		if name != "" {
			return nil, fmt.Errorf("actor needs a slug; put the display name in actor.name and the identity (e.g. claude:354bff2b) in actor.slug")
		}
		return nil, nil
	}
	if strings.ContainsAny(slug, " \t\n\r") {
		return nil, fmt.Errorf("actor.slug must be a stable slug like claude:354bff2b (no whitespace); the display name belongs in actor.name (got %q)", slug)
	}
	if len(slug) > maxActorSlugLen {
		return nil, fmt.Errorf("actor.slug must be at most %d characters (got %d)", maxActorSlugLen, len(slug))
	}
	return &ActorConfig{Slug: slug, Name: name, Kind: kind}, nil
}

// PersonSlug derives a stable person slug from a display name (GDK-1973):
// trim, collapse whitespace runs to single dashes, lowercase ASCII letters
// only (a non-ASCII name passes through unchanged — the name is the
// identity here), prefix "person:", and cut to maxActorSlugLen on a rune
// boundary. An empty name is the empty slug — the caller refuses a person
// with no name instead of storing a bare prefix.
func PersonSlug(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteByte('-')
		}
		for _, r := range f {
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			b.WriteRune(r)
		}
	}
	s := "person:" + b.String()
	// Byte cap (ValidateActor and issuetap measure bytes), cut on a rune
	// boundary so the result stays valid UTF-8.
	for len(s) > maxActorSlugLen {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}
