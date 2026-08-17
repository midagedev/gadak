package main

// gadak config — list / get / set profile settings through the same schema
// PUT /api/settings uses. Credentials stay on gadak init.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
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
	if err := cfg.Save(); err != nil {
		return err
	}
	return writeConfigValue(*asJSON, s.Path, s.Get(cfg))
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
