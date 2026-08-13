// Command seed-demo populates a throwaway Jira Cloud site with a realistic
// demo backlog for gadak screenshots and examples/demo.db.
//
//	export JIRA_SITE=https://your-site.atlassian.net
//	export JIRA_EMAIL=you@example.com
//	export JIRA_TOKEN=<api token from id.atlassian.com>
//	go run ./tools/seed-demo --data examples/demo-seed.json --projects NMB,NMA,NMS
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("seed-demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	projectsFlag := fs.String("projects", "NMB,NMA,NMS", "comma-separated project keys")
	issuesFlag := fs.Int("issues", 300, "total issues across all projects (procedural path)")
	seedFlag := fs.Int64("seed", 20260804, "RNG seed for procedural generation")
	dryRun := fs.Bool("dry-run", false, "print plan without mutating Jira")
	skipSetup := fs.Bool("skip-setup", false, "do not create versions/components")
	noHistory := fs.Bool("no-history", false, "create issues only; skip transitions, comments, links")
	dataPath := fs.String("data", "", "JSON dataset to project onto Jira instead of generating content")
	docsPath := fs.String("docs", "", "JSON wiki dataset to seed into Confluence (docs only; skips issue seeding)")
	epicsPath := fs.String("epics", "", "JSON epic hierarchy dataset to seed (epics only; skips issue seeding)")
	assigneesFlag := fs.String("assignees", "", "comma-separated accountIds for assignee slots")
	repairStatesFlag := fs.Bool("repair-states", false, "re-drive workflow states matched by summary")
	repairAssigneesFlag := fs.Bool("repair-assignees", false, "redistribute assignees across --assignees")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// --docs mode: Confluence wiki seed. dry-run never touches the network.
	if *docsPath != "" {
		if *dryRun {
			c := newClient("https://example.com", "dry@example.com", "dry-token")
			c.paceDelay = 0
			return c.seedDocs(*docsPath, true)
		}
		site := strings.TrimRight(os.Getenv("JIRA_SITE"), "/")
		email := os.Getenv("JIRA_EMAIL")
		token := os.Getenv("JIRA_TOKEN")
		if site == "" || email == "" || token == "" {
			fmt.Fprintln(os.Stderr, "JIRA_SITE, JIRA_EMAIL, and JIRA_TOKEN must be set")
			return 2
		}
		c := newClient(site, email, token)
		return c.seedDocs(*docsPath, false)
	}

	// --epics mode: create Epic issues and parent existing children by summary.
	// dry-run never touches the network.
	if *epicsPath != "" {
		if *dryRun {
			c := newClient("https://example.com", "dry@example.com", "dry-token")
			c.paceDelay = 0
			return c.seedEpics(*epicsPath, true)
		}
		site := strings.TrimRight(os.Getenv("JIRA_SITE"), "/")
		email := os.Getenv("JIRA_EMAIL")
		token := os.Getenv("JIRA_TOKEN")
		if site == "" || email == "" || token == "" {
			fmt.Fprintln(os.Stderr, "JIRA_SITE, JIRA_EMAIL, and JIRA_TOKEN must be set")
			return 2
		}
		c := newClient(site, email, token)
		return c.seedEpics(*epicsPath, false)
	}

	site := strings.TrimRight(os.Getenv("JIRA_SITE"), "/")
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_TOKEN")
	if site == "" || email == "" || token == "" {
		fmt.Fprintln(os.Stderr, "JIRA_SITE, JIRA_EMAIL, and JIRA_TOKEN must be set")
		return 2
	}

	rng := rand.New(rand.NewSource(*seedFlag))
	projects := splitCSV(*projectsFlag)
	for i, p := range projects {
		projects[i] = strings.ToUpper(p)
	}

	c := newClient(site, email, token)

	var me struct {
		AccountID    string `json:"accountId"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if !c.call("GET", "/rest/api/3/myself", nil, &me) {
		fmt.Fprintln(os.Stderr, "authentication failed")
		return 1
	}
	fmt.Printf("authenticated as %s <%s>\n", me.DisplayName, me.EmailAddress)

	if *repairStatesFlag || *repairAssigneesFlag {
		if *dataPath == "" {
			fmt.Fprintln(os.Stderr, "--repair-* requires --data")
			return 2
		}
		if *repairStatesFlag {
			c.repairStates(*dataPath, projects)
		}
		if *repairAssigneesFlag {
			pool := splitCSV(*assigneesFlag)
			if len(pool) == 0 {
				pool = []string{me.AccountID}
			}
			fmt.Printf("assignee pool: %d account(s)\n", len(pool))
			c.repairAssignees(*dataPath, projects, pool)
		}
		return 0
	}

	if *dataPath != "" {
		assignees := splitCSV(*assigneesFlag)
		if len(assignees) == 0 {
			assignees = []string{me.AccountID}
		}
		keys := c.seedFromData(*dataPath, projects, assignees, *dryRun, *skipSetup)
		fmt.Printf("\ndone. %d issues from %s\n", len(keys), *dataPath)
		return 0
	}

	// Procedural generation path.
	share := *issuesFlag / len(projects)
	if share < 1 {
		share = 1
	}
	var allKeys []string

	for _, project := range projects {
		profile, ok := projectProfiles[project]
		if !ok {
			fmt.Fprintf(os.Stderr, "no profile for %s, skipping\n", project)
			continue
		}
		fmt.Printf("\n[%s]\n", project)

		var versions, components []string
		if *skipSetup {
			versions, components = profile.versions, profile.components
		} else {
			versions = c.ensureVersions(project, profile.versions, *dryRun)
			components = c.ensureComponents(project, profile.components, *dryRun)
		}

		var types map[string]string
		if *dryRun {
			types = map[string]string{"Bug": "1", "Story": "2", "Task": "3"}
		} else {
			types = c.issueTypeIDs(project)
		}
		if len(types) == 0 {
			fmt.Fprintf(os.Stderr, "  could not read issue types for %s\n", project)
			continue
		}
		fmt.Printf("  issue types: %v\n", sortedKeys(types))

		payloads := buildIssues(project, profile, share, types, components, versions, me.AccountID, rng)
		created := c.createIssues(payloads, *dryRun)
		keys := make([]string, len(created))
		for i, ci := range created {
			keys[i] = ci.Key
		}
		allKeys = append(allKeys, keys...)

		if *dryRun || *noHistory {
			continue
		}

		moved := 0
		for _, k := range keys {
			moved += c.walkWorkflow(k, rng)
		}
		commented := 0
		for _, k := range keys {
			commented += c.addComments(k, rng)
		}
		fmt.Printf("  transitions: %d, comments: %d\n", moved, commented)
	}

	if !*dryRun && !*noHistory && len(allKeys) > 4 {
		pairs := len(allKeys) / 12
		if pairs < 4 {
			pairs = 4
		}
		links := c.linkIssues(allKeys, pairs, rng)
		fmt.Printf("\nlinks: %d\n", links)
	}

	fmt.Printf("\ndone. %d issues across %s\n", len(allKeys), strings.Join(projects, ", "))
	return 0
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
