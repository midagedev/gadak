package config

import (
	"fmt"
	"os"
	"strings"
)

// ActorConfig is the workspace-default acting identity for writes to an
// issuetap origin (localOrigin or paired, GDK-586): the slug becomes the
// origin accountId verbatim and names an agent account there; Name is an
// optional display name. Nil (or empty slug) means unset. Per-machine
// identity — never team-exported.
type ActorConfig struct {
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`

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
// Name the optional X-Issuetap-Actor-Name, Source which rung produced it.
type ResolvedActor struct {
	Slug   string `json:"slug"`
	Name   string `json:"name,omitempty"`
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
			return ResolvedActor{Slug: slug, Name: name, Source: ActorSourceEnv}, true
		}
		// An empty slug ("|name") is treated as unset, the same as Env()
		// treats an empty GADAK_* value: fall through, do not fail.
	}
	if cfg != nil && cfg.Actor != nil {
		if slug := strings.TrimSpace(cfg.Actor.Slug); slug != "" {
			return ResolvedActor{
				Slug:   slug,
				Name:   strings.TrimSpace(cfg.Actor.Name),
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
		return ResolvedActor{Slug: slug, Name: "Claude Code", Source: ActorSourceAuto}, true
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
// no whitespace, within issuetap's cap.
func ValidateActor(slug, name string) (*ActorConfig, error) {
	slug = strings.TrimSpace(slug)
	name = strings.TrimSpace(name)
	if slug == "" {
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
	return &ActorConfig{Slug: slug, Name: name}, nil
}
