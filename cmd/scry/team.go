package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/teamconfig"
)

func cmdTeam(args []string) error {
	if len(args) == 0 || wantsHelp(args) && (len(args) == 1 || args[0] == "-h" || args[0] == "--help") {
		printHelp("team")
		return nil
	}
	switch args[0] {
	case "export":
		return cmdTeamExport(args[1:])
	case "import":
		return cmdTeamImport(args[1:])
	default:
		return usageError("team", "usage: scry team export|import …")
	}
}

func cmdTeamExport(args []string) error {
	fs := newFlagSet("team")
	outPath := fs.String("out", "", "write to this file instead of stdout")
	withMembers := fs.Bool("with-members", false, "include members (emails) in the export")
	// Subcommand-specific usage: parent "team" help has no export flags.
	fs.Usage = func() {
		fmt.Print(`scry team export — write shareable team settings and saved views

Usage:
  scry [--profile <name>] team export [--out FILE] [--with-members]

Options:
  --out            write to this file instead of stdout
  --with-members   include members (emails) in the export

Examples:
  scry team export --out scry-team.json
  scry team export --with-members --out scry-team.json

See also: scry team import
`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	views, err := db.SavedViews()
	if err != nil {
		return err
	}

	if *withMembers {
		fmt.Fprintln(os.Stderr, "warning: --with-members includes email addresses in the export file")
	}

	doc := teamconfig.BuildDocument(cfg, views, teamconfig.ExportOptions{
		WithMembers: *withMembers,
	})
	raw, err := teamconfig.MarshalDocument(doc)
	if err != nil {
		return err
	}

	if *outPath == "" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	if err := os.WriteFile(*outPath, raw, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "exported team config to %s (%d settings keys, %d views)\n",
		*outPath, countSettingsKeys(doc.Settings), len(doc.Views))
	return nil
}

func countSettingsKeys(s teamconfig.TeamSettings) int {
	n := 0
	if len(s.Projects) > 0 {
		n++
	}
	if len(s.FieldMap) > 0 {
		n++
	}
	if len(s.BodyFields) > 0 {
		n++
	}
	if len(s.EditableFields) > 0 {
		n++
	}
	if len(s.Members) > 0 {
		n++
	}
	if len(s.GroupRules) > 0 {
		n++
	}
	if len(s.GroupLabels) > 0 {
		n++
	}
	if len(s.GroupColors) > 0 {
		n++
	}
	if len(s.ProductByGroup) > 0 {
		n++
	}
	if len(s.Features) > 0 {
		n++
	}
	if s.QaDashboardURL != "" {
		n++
	}
	if s.StaleThresholdHours != 0 {
		n++
	}
	return n
}

func cmdTeamImport(args []string) error {
	// Flags may appear before or after <FILE> (same ergonomics as scry sql / snapshot).
	const importUsage = "usage: scry team import <FILE|-> [--dry-run] [--overwrite]"
	dryRun := false
	overwrite := false
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Print(`scry team import — merge a team config file into this profile

Usage:
  scry [--profile <name>] team import <FILE|-> [--dry-run] [--overwrite]

Options:
  --dry-run     print what would change without writing
  --overwrite   replace conflicting settings keys and same-named views

Examples:
  scry team import scry-team.json --dry-run
  scry team import scry-team.json
  scry team import scry-team.json --overwrite
  cat scry-team.json | scry team import -

See also: scry team export
`)
			return nil
		case a == "--dry-run" || a == "-dry-run":
			dryRun = true
		case a == "--overwrite" || a == "-overwrite":
			overwrite = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 1 {
		return usageError("team", importUsage)
	}

	raw, err := readTeamFile(positionals[0])
	if err != nil {
		return err
	}
	doc, err := teamconfig.ParseDocument(raw)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Credentials must survive import. ApplyPlan never assigns these fields;
	// we still snapshot and restore them before Save as a hard guarantee.
	credSite, credEmail, credToken := cfg.Site, cfg.Email, cfg.Token
	credOwner, credVerified, credAcct := cfg.TokenOwner, cfg.TokenVerifiedAt, cfg.AccountID

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	views, err := db.SavedViews()
	if err != nil {
		return err
	}

	plan := teamconfig.BuildPlan(cfg, views, doc, teamconfig.ImportOptions{
		Overwrite: overwrite,
		DryRun:    dryRun,
	})

	fmt.Println("team import plan:")
	for _, line := range plan.SummaryLines() {
		fmt.Println("  " + line)
	}
	if dryRun {
		fmt.Println("dry-run: no changes written")
		fmt.Println("tip: re-run without --dry-run to apply; use --overwrite to replace conflicts")
		return nil
	}

	if err := teamconfig.ApplyPlan(cfg, db, plan); err != nil {
		return err
	}
	cfg.Site = credSite
	cfg.Email = credEmail
	cfg.Token = credToken
	cfg.TokenOwner = credOwner
	cfg.TokenVerifiedAt = credVerified
	cfg.AccountID = credAcct

	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("import applied")
	fmt.Println("tip: run with --dry-run first on a shared file to preview the merge plan")
	return nil
}

func readTeamFile(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}
