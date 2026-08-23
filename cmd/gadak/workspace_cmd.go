package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// workspaceUsage is the forms cmdWorkspace accepts (GDK-490).
const workspaceUsage = "usage: gadak workspace [--json] | gadak workspace use <name> | gadak workspace use --clear"

// workspaceView is the `gadak workspace --json` document.
type workspaceView struct {
	Workspace       string   `json:"workspace"`
	WorkspaceSource string   `json:"workspace_source"`
	Kind            string   `json:"kind"`
	Persist         string   `json:"persist"`
	Others          []string `json:"others"`
}

func cmdWorkspace(args []string) error {
	fs := newFlagSet("workspace")
	asJSON := fs.Bool("json", false, "emit JSON")
	clear := fs.Bool("clear", false, "with use: unset the stored default workspace")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("workspace", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		if *clear {
			return usageError("workspace", workspaceUsage)
		}
		return emitWorkspace(*asJSON)
	}
	if pos[0] != "use" {
		return usageError("workspace", workspaceUsage)
	}
	if *clear {
		if len(pos) != 1 {
			return usageError("workspace use", workspaceUsage)
		}
		if err := config.ClearStoredWorkspace(); err != nil {
			return err
		}
		return emitWorkspace(*asJSON)
	}
	if len(pos) != 2 {
		return usageError("workspace use", workspaceUsage)
	}
	if err := config.SetStoredWorkspace(pos[1]); err != nil {
		return err
	}
	return emitWorkspace(*asJSON)
}

func emitWorkspace(asJSON bool) error {
	doc, err := collectWorkspace()
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(doc)
	}
	printWorkspaceText(doc)
	return nil
}

func collectWorkspace() (workspaceView, error) {
	v := workspaceView{
		Workspace:       workspaceJSONName(),
		WorkspaceSource: workspaceJSONSource(),
		Others:          []string{},
	}
	if cfg, err := config.Load(); err == nil && cfg != nil {
		v.Kind = cfg.WorkspaceKind()
	}
	if dir, err := config.Dir(); err == nil {
		v.Persist = origin.PersistPath(dir)
	}
	active := v.Workspace
	if active != "default" {
		v.Others = append(v.Others, "default")
	}
	names, err := config.Profiles()
	if err != nil {
		return v, err
	}
	sort.Strings(names)
	for _, n := range names {
		if n == "" || n == active {
			continue
		}
		v.Others = append(v.Others, n)
	}
	return v, nil
}

func printWorkspaceText(v workspaceView) {
	fmt.Printf("%-18s %s\n", "workspace", v.Workspace)
	fmt.Printf("%-18s %s\n", "source", formatWorkspaceSource(v.WorkspaceSource))
	if v.Kind != "" {
		fmt.Printf("%-18s %s\n", "kind", v.Kind)
	}
	if v.Persist != "" {
		fmt.Printf("%-18s %s\n", "persist", v.Persist)
	}
	others := "(none)"
	if len(v.Others) > 0 {
		others = strings.Join(v.Others, ", ")
	}
	fmt.Printf("%-18s %s\n", "others", others)
	// The export hint is advice only when there is something else to pick:
	// on a single-workspace home it asks for work that changes nothing.
	if len(v.Others) > 0 {
		fmt.Printf("\nto keep using this workspace in this shell: export GADAK_WORKSPACE=%s\n", v.Workspace)
	}
}

func formatWorkspaceSource(src string) string {
	switch src {
	case config.SourceFlag, config.SourceDefault, config.SourceStored:
		return src
	default:
		return "env (" + src + ")"
	}
}

// workspaceJSONName is the display name for JSON "workspace" keys.
func workspaceJSONName() string {
	return displayProfileName(config.Profile())
}

// workspaceJSONSource is flag | default | <env var name>.
func workspaceJSONSource() string {
	kind, envName := config.WorkspaceSource()
	if kind == config.SourceEnv {
		return envName
	}
	if kind == "" {
		return config.SourceDefault
	}
	return kind
}

// warnWorkspaceIfEnv prints one stderr line when a write is going to a
// workspace that was selected by the environment (not visible in argv).
// stdout is untouched. Flag and root selections stay silent.
func warnWorkspaceIfEnv() {
	name := config.Profile()
	kind, envName := config.WorkspaceSource()
	if kind != config.SourceEnv || name == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: workspace: %s (from %s)\n", name, envName)
}
