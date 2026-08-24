package main

// gadak config — list / get / set profile settings through the same schema
// PUT /api/settings uses. Credentials stay on gadak init.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

const configUsage = "usage: gadak config [list|get <path>|set <path> <value>] [--json]"

const configCredentialNote = "credentials (site, email, token) are set with gadak init, not config set"

func cmdConfig(args []string) error {
	if wantsHelp(args) {
		printHelp("config")
		return nil
	}
	sub, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "get", "set":
			sub, rest = args[0], args[1:]
		default:
			return usageError("config", configUsage)
		}
	}
	switch sub {
	case "list":
		return configList(rest)
	case "get":
		return configGet(rest)
	case "set":
		return configSet(rest)
	default:
		return usageError("config", configUsage)
	}
}

type configListDoc struct {
	Settings []configListRow `json:"settings"`
	Note     string          `json:"note"`
}

type configListRow struct {
	Path        string `json:"path"`
	Value       any    `json:"value"`
	Description string `json:"description"`
}

func configList(args []string) error {
	fs := newFlagSet("config")
	asJSON := fs.Bool("json", false, "emit JSON")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rows := collectConfigRows(cfg)
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(configListDoc{
			Settings: rows,
			Note:     configCredentialNote,
		})
	}
	for _, r := range rows {
		val, err := json.Marshal(r.Value)
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\t%s\n", r.Path, val, r.Description)
	}
	fmt.Println("# credentials (site, email, token): gadak init")
	return nil
}

func configGet(args []string) error {
	fs := newFlagSet("config")
	asJSON := fs.Bool("json", false, "emit JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("config get", "usage: gadak config get <path> [--json]")
	}
	s, ok := config.SettingByPath(pos[0])
	if !ok {
		return unknownConfigPath(pos[0])
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return writeConfigValue(*asJSON, s.Path, s.Get(cfg))
}

func configSet(args []string) error {
	fs := newFlagSet("config")
	asJSON := fs.Bool("json", false, "emit JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageError("config set", "usage: gadak config set <path> <value> [--json]")
	}
	path := pos[0]
	rawText := strings.Join(pos[1:], " ")
	if rawText == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		rawText = strings.TrimSpace(string(b))
	}
	s, ok := config.SettingByPath(path)
	if !ok {
		return unknownConfigPath(path)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := s.Set(cfg, parseConfigValue(rawText)); err != nil {
		return err
	}
	if s.Path == "projects" {
		if err := checkProjectsOnSet(cfg); err != nil {
			return err
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	return writeConfigValue(*asJSON, s.Path, s.Get(cfg))
}

// checkProjectsOnSet is the GDK-809 site-membership choke for `config set
// projects`. Shape is already validated by config.ValidateProjectKeys.
// Origin reachable → unknown keys are refused with the catalog. Origin
// unreachable → warn against the mirror and still save (config set must
// work offline). Load of an existing file never calls this.
func checkProjectsOnSet(cfg *config.Config) error {
	if cfg == nil || len(cfg.Projects) == 0 {
		return nil
	}
	if !cfg.HasAtlassianCredential() {
		warnProjectsAgainstMirror(cfg, nil)
		return nil
	}
	keys, truncated, err := fetchSiteProjectKeys(cfg)
	if err != nil {
		warnProjectsAgainstMirror(cfg, err)
		return nil
	}
	have := make(map[string]bool, len(keys))
	for _, k := range keys {
		have[k] = true
	}
	var unknown []string
	for _, p := range cfg.Projects {
		if !have[p] {
			unknown = append(unknown, p)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "warning: project catalog was truncated; could not confirm %s is on the site (available sample: %s)\n", strings.Join(unknown, ", "), strings.Join(keys, ", "))
		return nil
	}
	if len(keys) == 0 {
		return fmt.Errorf("unknown project key %s — not on this site; origin returned no projects", strings.Join(unknown, ", "))
	}
	return fmt.Errorf("unknown project key %s — not on this site; available: %s", strings.Join(unknown, ", "), strings.Join(keys, ", "))
}

func fetchSiteProjectKeys(cfg *config.Config) ([]string, bool, error) {
	c, err := origin.Client(cfg)
	if err != nil {
		return nil, false, err
	}
	c.Retries = 1
	c.Backoff = 0
	if c.HTTP != nil {
		c.HTTP.Timeout = 4 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	list, truncated, err := c.Projects(ctx, 500)
	if err != nil {
		return nil, false, err
	}
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, p := range list {
		k := strings.ToUpper(strings.TrimSpace(p.Key))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Strings(out)
	return out, truncated, nil
}

func warnProjectsAgainstMirror(cfg *config.Config, originErr error) {
	db, err := openStore()
	if err != nil {
		if originErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not verify project keys against the site (%v); gadak status will report mismatches after sync\n", originErr)
		}
		return
	}
	defer db.Close()
	mirrored, merr := mirrorProjectKeys(db)
	if merr != nil || len(mirrored) == 0 {
		if originErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not verify project keys against the site (%v); gadak status will report mismatches after sync\n", originErr)
		}
		return
	}
	notMirrored, _ := projectScopeMismatch(cfg, db)
	if len(notMirrored) == 0 {
		return
	}
	msg := fmt.Sprintf("warning: configured, not in the mirror: %s (mirror has %s)", strings.Join(notMirrored, ", "), strings.Join(mirrored, ", "))
	if originErr != nil {
		msg += fmt.Sprintf(" — origin unreachable (%v)", originErr)
	}
	fmt.Fprintln(os.Stderr, msg)
}

func collectConfigRows(cfg *config.Config) []configListRow {
	all := config.Settings()
	rows := make([]configListRow, 0, len(all))
	for _, s := range all {
		rows = append(rows, configListRow{
			Path:        s.Path,
			Value:       s.Get(cfg),
			Description: s.Description,
		})
	}
	return rows
}

func writeConfigValue(asJSON bool, path string, val any) error {
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"path": path, "value": val})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(val)
}

func parseConfigValue(s string) json.RawMessage {
	s = strings.TrimSpace(s)
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}

func unknownConfigPath(path string) error {
	return &exitCodeError{
		code: 64,
		msg:  fmt.Sprintf("unknown config path %q — valid:\n  %s", path, strings.Join(config.SettingPaths(), "\n  ")),
	}
}

// exitCodeError lets a command pick its process status. unknown config paths
// use 64 (EX_USAGE); unknown flags stay 2 via unknownFlagErr.
type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string { return e.msg }
func (e *exitCodeError) ExitCode() int { return e.code }
