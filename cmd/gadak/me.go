package main

// `gadak me` (GDK-1973): a person says who they are on the built-in
// tracker. show prints the resolved identity the actor ladder answers with
// (GDK-586 — env > config > Claude Code detection > nobody); set writes
// the config actor block as a person through the same settings-registry
// setter `gadak config set actor` uses, so the person rule and the trailer
// preservation have one owner; clear removes the block. On a connected
// Jira/Linear workspace set is refused: there the Atlassian account is
// the identity, and only agents have a reason to stamp a different one
// (`gadak config set actor`).

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

func cmdMe(args []string) error {
	if wantsHelp(args) {
		printHelp("me")
		return nil
	}
	fs := newFlagSet("me")
	asJSON := fs.Bool("json", false, "emit JSON (show)")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	sub := ""
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "":
		return meShow(*asJSON)
	case "set":
		if len(pos) < 2 {
			return usageError("me set", "usage: gadak me set \"Your Name\"")
		}
		return meSet(strings.Join(pos[1:], " "))
	case "clear":
		if len(pos) > 1 {
			return usageError("me clear", "usage: gadak me clear")
		}
		return meClear()
	default:
		return usageError("me", fmt.Sprintf("unknown subcommand %q — usage: gadak me [set \"Your Name\"|clear] [--json]", sub))
	}
}

// meDoc is `gadak me --json`. source is the ladder rung that answered, or
// "none" (config has no such const: no rung is not a source). kind is
// agent or person, never empty when a rung answered.
// default_author is filled only when nothing resolved and the origin can
// name its own default user cheaply (local built-in) — the name init's
// success line prints, from the same helper.
type meDoc struct {
	Name          string `json:"name,omitempty"`
	Slug          string `json:"slug"`
	Kind          string `json:"kind,omitempty"`
	Source        string `json:"source"`
	Workspace     string `json:"workspace"`
	DefaultAuthor string `json:"default_author,omitempty"`
}

func meShow(asJSON bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	actor, resolved := config.ResolveActor(cfg)
	doc := meDoc{
		Slug:      actor.Slug,
		Workspace: workspaceJSONName(),
	}
	if resolved {
		doc.Name, doc.Kind, doc.Source = actor.Name, actor.Kind, actor.Source
	} else {
		doc.Source = "none"
		// Ask the origin only when it is in this process (no dial, no
		// paired round trip): on a connected workspace the account is the
		// identity anyway, and a paired one would stall `me` on the home
		// serve for a name the sentence can live without.
		if cfg != nil && cfg.OriginType() == config.OriginGadak && cfg.Transport() == config.TransportLocal {
			doc.DefaultAuthor = builtInAuthorName(cfg)
		}
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(doc)
	}
	if resolved {
		line := actor.Slug
		if actor.Name != "" && actor.Name != actor.Slug {
			line += " — " + actor.Name
		}
		fmt.Printf("%-16s %s\n", "identity", line)
		fmt.Printf("%-16s %s\n", "kind", actor.Kind)
		fmt.Printf("%-16s %s\n", "source", actor.Source)
		fmt.Printf("%-16s %s\n", "workspace", doc.Workspace)
		return nil
	}
	if cfg != nil && cfg.OriginType() == config.OriginGadak {
		author := "the origin's default user"
		if doc.DefaultAuthor != "" {
			author = doc.DefaultAuthor + " (the origin's default user)"
		}
		fmt.Printf("no actor is set — writes are authored as %s\n", author)
		fmt.Println(`say who you are: gadak me set "Your Name"`)
		fmt.Println("agents: GADAK_ACTOR, or gadak config set actor")
		return nil
	}
	fmt.Println("no actor is set — on a connected workspace the account is the identity, and writes carry it as-is")
	fmt.Println("agents: GADAK_ACTOR, or gadak config set actor")
	return nil
}

func meSet(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.OriginType() != config.OriginGadak {
		return fmt.Errorf("me set: this workspace's origin is %s — there the account is the identity; agents use `gadak config set actor`", cfg.OriginType())
	}
	s, ok := config.SettingByPath("actor")
	if !ok {
		return unknownConfigPath("actor")
	}
	// The person block, through the settings registry's own setter: the
	// person rule (slug derived from the name, blank name refused) and the
	// trailer preservation stay in one owner. The trailer the current
	// block carries rides along — a person's `me set` must not silently
	// flip the attribution line back on.
	in := config.ActorConfig{Name: strings.TrimSpace(name), Kind: config.ActorKindPerson}
	if cfg.Actor != nil {
		in.Trailer = cfg.Actor.Trailer
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	// A higher rung answering this session would mask the block — say so on
	// stderr, where it cannot disturb the stdout contract.
	if a, ok := config.ResolveActor(cfg); ok && a.Source != config.ActorSourceConfig {
		fmt.Fprintf(os.Stderr, "note: this session writes as %s (the %s rung) — the stored name applies where that is not set\n", a.Slug, a.Source)
	}
	if err := s.Set(cfg, raw); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("you are %s (%s) — this workspace's writes are authored as you\n", cfg.Actor.Name, cfg.Actor.Slug)
	return nil
}

func meClear() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s, ok := config.SettingByPath("actor")
	if !ok {
		return unknownConfigPath("actor")
	}
	// The same clear `config set actor ""` performs — an empty object
	// through the same setter, not a direct cfg.Actor = nil, so the
	// registry stays the single writer of the block.
	if err := s.Set(cfg, json.RawMessage(`{}`)); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("cleared — writes are authored as the origin's default user again")
	return nil
}
