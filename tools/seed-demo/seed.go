package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
)

func (c *Client) ensureVersions(project string, names []string, dry bool) []string {
	made := make([]string, 0, len(names))
	for index, name := range names {
		released := index < len(names)-2
		if dry {
			made = append(made, name)
			continue
		}
		var res map[string]any
		ok := c.call("POST", "/rest/api/3/version", map[string]any{
			"name":        name,
			"project":     project,
			"released":    released,
			"description": project + " release " + name,
		}, &res)
		if ok {
			made = append(made, name)
		}
	}
	fmt.Printf("  versions: %v\n", made)
	return made
}

func (c *Client) ensureComponents(project string, names []string, dry bool) []string {
	made := make([]string, 0, len(names))
	for _, name := range names {
		if dry {
			made = append(made, name)
			continue
		}
		var res map[string]any
		ok := c.call("POST", "/rest/api/3/component", map[string]any{
			"name":    name,
			"project": project,
		}, &res)
		if ok {
			made = append(made, name)
		}
	}
	fmt.Printf("  components: %v\n", made)
	return made
}

func buildIssues(project string, profile projectProfile, count int, types map[string]string,
	components, versions []string, me string, rng *rand.Rand) []map[string]any {

	issues := make([]map[string]any, 0, count)
	typeNames := sortedKeys(types)
	for range count {
		kind := weightedPick(rng, profile.typeWeights)
		if _, ok := types[kind]; !ok {
			// Project does not offer this type — fall back to any available one.
			kind = typeNames[0]
		}
		area := profile.areas[rng.Intn(len(profile.areas))]
		patPool := patterns[kind]
		if len(patPool) == 0 {
			patPool = taskPatterns
		}
		summary := formatArea(patPool[rng.Intn(len(patPool))], area)
		if len(summary) > 250 {
			summary = summary[:250]
		}
		fields := map[string]any{
			"project":     map[string]string{"key": project},
			"issuetype":   map[string]string{"id": types[kind]},
			"summary":     summary,
			"priority":    map[string]string{"name": weightedPick(rng, priorityWeights)},
			"description": plainDescription(summary),
		}
		if kind == "Bug" {
			fields["description"] = bugDescription(area)
		}
		if rng.Float64() < 0.7 && len(components) > 0 {
			k := 1
			if len(components) > 1 && rng.Intn(2) == 1 {
				k = 2
				if k > len(components) {
					k = len(components)
				}
			}
			picks := sampleStrings(rng, components, k)
			comps := make([]map[string]string, len(picks))
			for i, cname := range picks {
				comps[i] = map[string]string{"name": cname}
			}
			fields["components"] = comps
		}
		if rng.Float64() < 0.55 && len(versions) > 0 {
			fields["fixVersions"] = []map[string]string{{"name": versions[rng.Intn(len(versions))]}}
		}
		if rng.Float64() < 0.6 {
			k := rng.Intn(3) + 1
			fields["labels"] = sampleStrings(rng, labelPool, k)
		}
		if kind == "Bug" && rng.Float64() < 0.7 {
			fields["environment"] = adf([]string{environments[rng.Intn(len(environments))]})
		}
		// Leave a realistic share unassigned; the rest go to the seeding account.
		if rng.Float64() < 0.45 {
			fields["assignee"] = map[string]string{"id": me}
		}
		issues = append(issues, map[string]any{"fields": fields})
	}
	return issues
}

func sampleStrings(rng *rand.Rand, pool []string, k int) []string {
	if k > len(pool) {
		k = len(pool)
	}
	// Fisher-Yates partial shuffle copy.
	idx := make([]int, len(pool))
	for i := range idx {
		idx[i] = i
	}
	for i := 0; i < k; i++ {
		j := i + rng.Intn(len(pool)-i)
		idx[i], idx[j] = idx[j], idx[i]
	}
	out := make([]string, k)
	for i := 0; i < k; i++ {
		out[i] = pool[idx[i]]
	}
	return out
}

func (c *Client) createIssues(payloads []map[string]any, dry bool) []createdIssue {
	created := make([]createdIssue, 0, len(payloads))
	for start := 0; start < len(payloads); start += 50 {
		end := start + 50
		if end > len(payloads) {
			end = len(payloads)
		}
		batch := payloads[start:end]
		if dry {
			for i := range batch {
				created = append(created, createdIssue{ID: "0", Key: fmt.Sprintf("DRY-%d", start+i)})
			}
			continue
		}
		var res struct {
			Issues []createdIssue   `json:"issues"`
			Errors []map[string]any `json:"errors"`
		}
		if c.call("POST", "/rest/api/3/issue/bulk", map[string]any{"issueUpdates": batch}, &res) {
			created = append(created, res.Issues...)
			for i, err := range res.Errors {
				if i >= 3 {
					break
				}
				fmt.Fprintf(os.Stderr, "  create error: %v\n", truncate(fmt.Sprintf("%v", err), 300))
			}
		}
		fmt.Printf("  created %d/%d\n", len(created), len(payloads))
	}
	return created
}

type createdIssue struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (c *Client) seedFromData(path string, projects, assignees []string, dry, skipSetup bool) []string {
	data, err := loadDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ERROR load dataset: %v\n", err)
		return nil
	}
	items := filterIssues(data.Issues, projects)
	fmt.Printf("dataset: %d issues for %s\n", len(items), strings.Join(projects, ", "))

	perProject := map[string]projectTypeStatus{}
	for _, project := range projects {
		profile := projectProfiles[project]
		if !skipSetup && !dry {
			c.ensureVersions(project, profile.versions, dry)
			c.ensureComponents(project, profile.components, dry)
		}
		pts := projectTypeStatus{
			types:    c.issueTypeIDs(project),
			statuses: c.projectStatusIDs(project),
		}
		perProject[project] = pts
		printTypeStatus(project, pts)
	}

	// Build payloads in dataset order so link indexes stay meaningful.
	payloads := make([]map[string]any, 0, len(items))
	order := make([]SeedIssue, 0, len(items))
	for index, item := range items {
		meta := perProject[item.Project]
		typeID := meta.types[item.Type]
		if typeID == "" {
			warnSkip(index, item.Type, item.Project)
			continue
		}
		summary := item.Summary
		if len(summary) > 250 {
			summary = summary[:250]
		}
		desc := item.Description
		if len(desc) == 0 {
			desc = []string{item.Summary}
		}
		fields := map[string]any{
			"project":     map[string]string{"key": item.Project},
			"issuetype":   map[string]string{"id": typeID},
			"summary":     summary,
			"description": adf(desc),
		}
		if item.Priority != "" {
			fields["priority"] = map[string]string{"name": item.Priority}
		}
		if len(item.Components) > 0 {
			comps := make([]map[string]string, len(item.Components))
			for i, name := range item.Components {
				comps[i] = map[string]string{"name": name}
			}
			fields["components"] = comps
		}
		if item.FixVersion != nil && *item.FixVersion != "" {
			fields["fixVersions"] = []map[string]string{{"name": *item.FixVersion}}
		}
		if len(item.Labels) > 0 {
			fields["labels"] = item.Labels
		}
		if item.Environment != nil && *item.Environment != "" {
			fields["environment"] = adf([]string{*item.Environment})
		}
		if item.AssigneeSlot != nil && len(assignees) > 0 {
			fields["assignee"] = map[string]string{"id": assignees[*item.AssigneeSlot%len(assignees)]}
		}
		payloads = append(payloads, map[string]any{"fields": fields})
		order = append(order, item)
	}

	created := c.createIssues(payloads, dry)
	if dry {
		keys := make([]string, len(created))
		for i, ci := range created {
			keys[i] = ci.Key
		}
		return keys
	}

	keys := make([]string, len(created))
	for i, ci := range created {
		keys[i] = ci.Key
	}
	if len(keys) != len(order) {
		fmt.Fprintf(os.Stderr, "  WARNING: created %d of %d — links may shift\n", len(keys), len(order))
	}

	// Status, then comments, then links: comments land after the transitions
	// they narrate, and links need every key to exist.
	moved, reopened := c.applyStates(keys, order, perProject)
	fmt.Printf("  transitions: %d, reopened: %d\n", moved, reopened)

	commented := 0
	for i, key := range keys {
		if i >= len(order) {
			break
		}
		for _, text := range order[i].Comments {
			if c.call("POST", "/rest/api/3/issue/"+url.PathEscape(key)+"/comment",
				map[string]any{"body": adf([]string{text})}, nil) {
				commented++
			}
		}
	}
	fmt.Printf("  comments: %d\n", commented)

	linked := 0
	for i, key := range keys {
		if i >= len(order) {
			break
		}
		for _, link := range order[i].Links {
			target := link.Target
			if target < 0 || target >= len(keys) {
				continue
			}
			if keys[target] == key {
				continue
			}
			typ := link.Type
			if typ == "" {
				typ = "Relates"
			}
			if c.call("POST", "/rest/api/3/issueLink", map[string]any{
				"type":         map[string]string{"name": typ},
				"inwardIssue":  map[string]string{"key": key},
				"outwardIssue": map[string]string{"key": keys[target]},
			}, nil) {
				linked++
			}
		}
	}
	fmt.Printf("  links: %d\n", linked)
	return keys
}

func (c *Client) addComments(issueKey string, rng *rand.Rand) int {
	count := weightedInt(rng, []int{0, 1, 2, 3}, []int{45, 30, 17, 8})
	if count > len(commentPool) {
		count = len(commentPool)
	}
	texts := sampleStrings(rng, commentPool, count)
	for _, text := range texts {
		c.call("POST", "/rest/api/3/issue/"+url.PathEscape(issueKey)+"/comment",
			map[string]any{"body": adf([]string{text})}, nil)
	}
	return count
}

func (c *Client) linkIssues(keys []string, pairs int, rng *rand.Rand) int {
	if len(keys) < 2 {
		return 0
	}
	made := 0
	for range pairs {
		i := rng.Intn(len(keys))
		j := rng.Intn(len(keys) - 1)
		if j >= i {
			j++
		}
		a, b := keys[i], keys[j]
		typ := linkTypes[rng.Intn(len(linkTypes))]
		if c.call("POST", "/rest/api/3/issueLink", map[string]any{
			"type":         map[string]string{"name": typ},
			"inwardIssue":  map[string]string{"key": a},
			"outwardIssue": map[string]string{"key": b},
		}, nil) {
			made++
		}
	}
	return made
}
