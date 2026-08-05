package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// seedDocs loads a wiki dataset and creates Confluence spaces, pages, and
// comments. dry prints a plan and never hits the network.
func (c *Client) seedDocs(path string, dry bool) int {
	data, err := loadDocsDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR load docs dataset: %v\n", err)
		return 1
	}
	return c.seedDocsData(data, dry)
}

func (c *Client) seedDocsData(data *DocsDataset, dry bool) int {
	ordered, err := topoPages(data.Pages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR page order: %v\n", err)
		return 1
	}

	commentTotal := 0
	for _, p := range ordered {
		commentTotal += len(p.Comments)
	}

	fmt.Printf("docs plan: %d spaces, %d pages, %d comments\n",
		len(data.Spaces), len(ordered), commentTotal)
	for _, s := range data.Spaces {
		fmt.Printf("  space %s — %s\n", s.Key, s.Name)
	}
	bySpace := map[string]int{}
	for _, p := range ordered {
		bySpace[p.Space]++
		parentNote := ""
		if p.Parent != "" {
			parentNote = " (parent: " + p.Parent + ")"
		}
		fmt.Printf("  page [%s] %s%s  comments=%d\n", p.Space, p.Title, parentNote, len(p.Comments))
	}
	if dry {
		fmt.Printf("dry-run: no Confluence calls\n")
		return 0
	}

	// Ensure spaces.
	for _, s := range data.Spaces {
		if !c.ensureSpace(s.Key, s.Name) {
			fmt.Fprintf(os.Stderr, "ERROR ensure space %s\n", s.Key)
			return 1
		}
		c.pace()
	}

	// page title → content id within a space
	ids := make(map[string]string) // space+"\x00"+title → id
	created, skipped, commentsMade := 0, 0, 0

	for _, p := range ordered {
		key := p.Space + "\x00" + p.Title
		if id, ok := c.findPage(p.Space, p.Title); ok && id != "" {
			fmt.Printf("  skip existing page [%s] %s (id %s)\n", p.Space, p.Title, id)
			ids[key] = id
			skipped++
			c.pace()
			continue
		}
		c.pace()

		var parentID string
		if p.Parent != "" {
			parentID = ids[p.Space+"\x00"+p.Parent]
			if parentID == "" {
				fmt.Fprintf(os.Stderr, "ERROR missing parent id for %q under %q\n", p.Title, p.Parent)
				return 1
			}
		}
		id, ok := c.createPage(p.Space, p.Title, parentID, p.BodyStorage)
		if !ok || id == "" {
			fmt.Fprintf(os.Stderr, "ERROR create page [%s] %s\n", p.Space, p.Title)
			return 1
		}
		ids[key] = id
		created++
		fmt.Printf("  created page [%s] %s (id %s)\n", p.Space, p.Title, id)
		c.pace()

		for _, cm := range p.Comments {
			if c.createComment(id, cm.BodyStorage) {
				commentsMade++
			}
			c.pace()
		}
	}

	fmt.Printf("done. pages created=%d skipped=%d comments=%d\n", created, skipped, commentsMade)
	return 0
}

// topoPages returns pages in parent-before-child order. Detects missing parents
// and cycles.
func topoPages(pages []DocsPage) ([]DocsPage, error) {
	type key struct{ space, title string }
	byKey := make(map[key]*DocsPage, len(pages))
	for i := range pages {
		p := &pages[i]
		k := key{p.Space, p.Title}
		if _, ok := byKey[k]; ok {
			return nil, fmt.Errorf("duplicate page %s / %s", p.Space, p.Title)
		}
		byKey[k] = p
	}
	for _, p := range pages {
		if p.Parent == "" {
			continue
		}
		if _, ok := byKey[key{p.Space, p.Parent}]; !ok {
			return nil, fmt.Errorf("page %q parent %q missing in space %s", p.Title, p.Parent, p.Space)
		}
	}

	// Kahn: indegree = 1 if has parent else 0; edge parent → child.
	children := make(map[key][]key)
	indeg := make(map[key]int, len(pages))
	for k := range byKey {
		indeg[k] = 0
	}
	for k, p := range byKey {
		if p.Parent == "" {
			continue
		}
		pk := key{p.Space, p.Parent}
		children[pk] = append(children[pk], k)
		indeg[k]++
	}

	// Stable queue: original order among ready nodes.
	var queue []key
	seen := make(map[key]bool)
	for i := range pages {
		k := key{pages[i].Space, pages[i].Title}
		if indeg[k] == 0 && !seen[k] {
			queue = append(queue, k)
			seen[k] = true
		}
	}

	out := make([]DocsPage, 0, len(pages))
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		out = append(out, *byKey[k])
		for _, ch := range children[k] {
			indeg[ch]--
			if indeg[ch] == 0 {
				queue = append(queue, ch)
			}
		}
	}
	if len(out) != len(pages) {
		return nil, fmt.Errorf("parent cycle detected (%d of %d ordered)", len(out), len(pages))
	}
	return out, nil
}

func (c *Client) pace() {
	if c.paceDelay <= 0 {
		return
	}
	time.Sleep(c.paceDelay)
}

func (c *Client) ensureSpace(key, name string) bool {
	// GET existing
	var existing map[string]any
	if c.call("GET", "/wiki/rest/api/space/"+url.PathEscape(key), nil, &existing) {
		fmt.Printf("  space %s exists\n", key)
		return true
	}
	// create
	var created map[string]any
	ok := c.call("POST", "/wiki/rest/api/space", map[string]any{
		"key":  key,
		"name": name,
	}, &created)
	if ok {
		fmt.Printf("  space %s created\n", key)
	}
	return ok
}

func (c *Client) findPage(spaceKey, title string) (id string, ok bool) {
	q := url.Values{}
	q.Set("spaceKey", spaceKey)
	q.Set("title", title)
	q.Set("type", "page")
	q.Set("status", "current")
	var res struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if !c.call("GET", "/wiki/rest/api/content?"+q.Encode(), nil, &res) {
		return "", false
	}
	if len(res.Results) == 0 {
		return "", true // call ok, not found
	}
	return res.Results[0].ID, true
}

func (c *Client) createPage(spaceKey, title, parentID, body string) (id string, ok bool) {
	payload := map[string]any{
		"type":  "page",
		"title": title,
		"space": map[string]string{"key": spaceKey},
		"body": map[string]any{
			"storage": map[string]string{
				"value":          body,
				"representation": "storage",
			},
		},
	}
	if parentID != "" {
		payload["ancestors"] = []map[string]string{{"id": parentID}}
	}
	var res struct {
		ID string `json:"id"`
	}
	if !c.call("POST", "/wiki/rest/api/content", payload, &res) {
		return "", false
	}
	return res.ID, true
}

func (c *Client) createComment(pageID, body string) bool {
	payload := map[string]any{
		"type": "comment",
		"container": map[string]string{
			"id":   pageID,
			"type": "page",
		},
		"body": map[string]any{
			"storage": map[string]string{
				"value":          body,
				"representation": "storage",
			},
		},
	}
	return c.call("POST", "/wiki/rest/api/content", payload, nil)
}

// parseRetryAfter returns a wait duration from a Retry-After header value
// (seconds or HTTP-date). Zero means "use default backoff".
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if sec, err := strconv.Atoi(h); err == nil && sec >= 0 {
		return time.Duration(sec) * time.Second
	}
	if t, err := httpParseDate(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func httpParseDate(s string) (time.Time, error) {
	// RFC 1123 is what servers usually send.
	return time.Parse(time.RFC1123, s)
}
