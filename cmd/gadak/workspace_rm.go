package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/workspace"
)

// workspaceRMUsage is the shape refusal for `workspaces rm`.
const workspaceRMUsage = "usage: gadak workspaces rm <name> [--yes] [--destroy-origin] [--json]"

// workspaceRMDoc is the `--json` success document: exactly what was removed
// and whether the local-origin origin died with it.
type workspaceRMDoc struct {
	Removed         string `json:"removed"`
	Kind            string `json:"kind"`
	OriginDestroyed bool   `json:"origin_destroyed"`
}

// removeWorkspace implements `gadak workspaces rm <name>` (same verb through
// the `profiles` alias). The removal contract — refusals, ordering, the
// stored-default cleanup, the pairing pre-read — lives in
// workspace.Remove, shared with DELETE /api/v1/workspaces/{name}; this is
// the CLI shell: flags, the exit wording, and human/JSON output. The
// refusal texts are Remove's (byte-preserved from the pre-extraction verb).
func removeWorkspace(invoked string, args []string) error {
	if wantsHelp(args) {
		printHelp(invoked)
		return nil
	}
	fs := newFlagSet(invoked + " rm")
	yes := fs.Bool("yes", false, "remove the workspace — without this, rm explains and refuses")
	destroyOrigin := fs.Bool("destroy-origin", false, "local-origin only: also destroy this profile's persist, the only copy of that tracker")
	asJSON := fs.Bool("json", false, "emit JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || strings.TrimSpace(pos[0]) == "" {
		return usageError(invoked+" rm", workspaceRMUsage)
	}
	name := strings.TrimSpace(pos[0])

	res, err := workspace.Remove(name, *yes, *destroyOrigin, invoked)
	if err != nil {
		return err
	}

	doc := workspaceRMDoc{Removed: res.Removed, Kind: res.Kind, OriginDestroyed: res.OriginDestroyed}
	if *asJSON {
		if err := json.NewEncoder(os.Stdout).Encode(doc); err != nil {
			return err
		}
		// Advisories go to stderr so stdout stays exactly the document.
		for _, line := range res.Advisories() {
			fmt.Fprintln(os.Stderr, line)
		}
		return nil
	}

	fmt.Printf("removed workspace %q (%s) — %s\n", name, res.Kind, res.Dir)
	if doc.OriginDestroyed {
		fmt.Printf("destroyed local-origin origin: %s\n", res.Persist)
	} else if res.LocalOrigin {
		fmt.Println("no local-origin persist was present; no origin data existed to destroy")
	} else {
		fmt.Println("the origin is untouched: only this machine's mirror and credential existed here")
	}
	for _, line := range res.Advisories() {
		fmt.Fprintln(os.Stdout, line)
	}
	return nil
}
