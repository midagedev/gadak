package jira

import (
	"encoding/json"
	"sort"
	"strings"
)

// Doc builds the ADF document a comment body has to be. The composer sends plain
// text with `@Display Name` typed into it plus the account ids it resolved, so
// this is where those become real mention nodes — a mention that stays plain text
// notifies nobody, which is the whole point of typing it.
//
// mentions maps display name to account id.
func Doc(text string, mentions map[string]string) json.RawMessage {
	return DocWithMedia(text, mentions, nil)
}

// Media is one inline image in a comment: the Jira media UUID (not the
// attachment id — see Client.MediaRef) plus the filename, which is carried as
// `alt` so our own renderer can match the node to the attachment without
// persisting the UUID anywhere (web/src/lib/adf.ts, findAttachment).
type Media struct {
	ID       string
	Filename string
}

// DocWithMedia is Doc plus inline images, appended after the text — where a
// screenshot belongs in a comment that describes it.
func DocWithMedia(text string, mentions map[string]string, media []Media) json.RawMessage {
	// Longest name first: "@김현" must not win over "@김현철".
	names := make([]string, 0, len(mentions))
	for name := range mentions {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	content := make([]any, 0, len(lines))
	for _, line := range lines {
		para := map[string]any{"type": "paragraph"}
		if nodes := inline(line, names, mentions); len(nodes) > 0 {
			para["content"] = nodes
		}
		content = append(content, para)
	}
	for _, m := range media {
		if m.ID == "" {
			continue
		}
		// collection must be present and empty for an issue attachment; Jira
		// rejects the node without it.
		attrs := map[string]any{"type": "file", "id": m.ID, "collection": ""}
		if m.Filename != "" {
			attrs["alt"] = m.Filename
		}
		content = append(content, map[string]any{
			"type":    "mediaSingle",
			"attrs":   map[string]any{"layout": "center"},
			"content": []any{map[string]any{"type": "media", "attrs": attrs}},
		})
	}
	doc, err := json.Marshal(map[string]any{"type": "doc", "version": 1, "content": content})
	if err != nil {
		return json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	}
	return doc
}

func inline(line string, names []string, ids map[string]string) []any {
	nodes := []any{}
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			nodes = append(nodes, map[string]any{"type": "text", "text": buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(line); {
		if line[i] == '@' {
			if name := match(line[i+1:], names); name != "" {
				flush()
				nodes = append(nodes, map[string]any{
					"type":  "mention",
					"attrs": map[string]any{"id": ids[name], "text": "@" + name},
				})
				i += 1 + len(name)
				continue
			}
		}
		buf.WriteByte(line[i])
		i++
	}
	flush()
	return nodes
}

func match(rest string, names []string) string {
	for _, name := range names {
		if strings.HasPrefix(rest, name) {
			return name
		}
	}
	return ""
}
