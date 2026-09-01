package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// cmdProject is the project partition surface (GDK-391). Creation is
// local-origin-only: the embedded origin grows a project through its own
// Jira API (writes pass through the origin, never the mirror). On a
// connected workspace projects are Jira admin territory — gadak refuses
// and points there instead of half-owning the verb.
func cmdProject(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("project", nil))
		return nil
	}
	switch args[0] {
	case "create":
		return cmdProjectCreate(args[1:])
	default:
		return fmt.Errorf("project: unknown subcommand %q (try `gadak project create`)", args[0])
	}
}

func cmdProjectCreate(args []string) error {
	fs := newFlagSet("project create")
	name := fs.String("name", "", "display name (omitted = the key)")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("project", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("project create: exactly one project key (usage: gadak project create <KEY> [--name N])")
	}
	key := rest[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// No-origin homes answer the shared init sentence (GDK-943); the
	// local-origin-only refusal below is only for workspaces that exist.
	if !cfg.HasOrigin() {
		return config.NotConfiguredWith("project create writes to the origin, not to the mirror")
	}
	if !cfg.HasLocalOrigin() {
		return fmt.Errorf("project create is for local-origin workspaces — on a connected workspace, create the project in Jira and run `gadak sync`")
	}
	client, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"key": key, "name": *name})
	if err != nil {
		return err
	}
	status, out, err := client.Raw(context.Background(), "POST", "/rest/api/3/project", body, true)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("project create: origin returned HTTP %d: %s", status, out)
	}
	if *asJSON {
		fmt.Println(string(out))
		return nil
	}
	var created struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		return err
	}
	fmt.Printf("%s\tproject created (id %s) — `gadak create --project %s <SUMMARY>` starts it\n", created.Key, created.ID, created.Key)
	return nil
}
