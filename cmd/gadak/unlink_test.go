package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Contract ↔ assertion (GDK-1205, the reverse of the GDK-19 link half):
//
//  1. --type blocks deletes the link displayed on A as "A blocks B" — the
//     outwardIssue element — by the id the live GET handed out
//     TestUnlinkDeletesTheOutwardDisplayedLink
//  2. --type "is blocked by" matches the inwardIssue element instead
//     TestUnlinkInwardDescriptionMatchesInwardElement
//  3. no matching element → error naming the phrase; nothing DELETEd
//     TestUnlinkNoMatchDeletesNothing
//  4. built-in: link then unlink through the in-process issuetap removes
//     both projections from the mirror — the cross-repo pin for the
//     synthetic id contract
//     TestUnlinkBuiltInRemovesBothProjections

const unlinkLinksJSON = `{"fields":{
	"status":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},
	"issuelinks":[
		{"id":"10500","type":{"id":"10000","name":"Blocks","outward":"blocks","inward":"is blocked by"},"outwardIssue":{"key":"NMB-2"}},
		{"id":"10501","type":{"id":"10000","name":"Blocks","outward":"blocks","inward":"is blocked by"},"inwardIssue":{"key":"NMB-3"}}
	]}}`

func TestUnlinkDeletesTheOutwardDisplayedLink(t *testing.T) {
	f := newFakeJira(t)
	f.issueStatusJSON = unlinkLinksJSON
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdUnlink([]string{"NMB-1", "NMB-2", "--type", "blocks"})
	})
	if err != nil {
		t.Fatalf("unlink: %v\n%s", err, out)
	}
	if len(f.deletedLinks) != 1 || f.deletedLinks[0] != "10500" {
		t.Fatalf("deleted %v, want [10500]", f.deletedLinks)
	}
}

func TestUnlinkInwardDescriptionMatchesInwardElement(t *testing.T) {
	f := newFakeJira(t)
	f.issueStatusJSON = unlinkLinksJSON
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdUnlink([]string{"NMB-1", "NMB-3", "--type", "is blocked by"})
	})
	if err != nil {
		t.Fatalf("unlink: %v\n%s", err, out)
	}
	if len(f.deletedLinks) != 1 || f.deletedLinks[0] != "10501" {
		t.Fatalf("deleted %v, want [10501]", f.deletedLinks)
	}
}

func TestUnlinkNoMatchDeletesNothing(t *testing.T) {
	f := newFakeJira(t)
	f.issueStatusJSON = unlinkLinksJSON
	mirror(t, f.URL)

	// NMB-3 is linked, but as "is blocked by" — the outward phrase finds
	// nothing, and the error says which phrase it looked for.
	_, err := capture(t, func() error {
		return cmdUnlink([]string{"NMB-1", "NMB-3", "--type", "blocks"})
	})
	if err == nil || !strings.Contains(err.Error(), "NMB-1 blocks NMB-3") {
		t.Fatalf("err = %v, want the missing displayed phrase named", err)
	}
	if len(f.deletedLinks) != 0 {
		t.Fatalf("deleted %v, want none", f.deletedLinks)
	}
}

func TestUnlinkBuiltInRemovesBothProjections(t *testing.T) {
	builtInHome(t)
	a := createBuiltIn(t, "unlink a")
	b := createBuiltIn(t, "unlink b")
	if _, err := capture(t, func() error {
		return cmdLink([]string{a, b, "--type", "blocks"})
	}); err != nil {
		t.Fatalf("link: %v", err)
	}
	if n := mirrorLinkRows(t, a, b); n != 2 {
		t.Fatalf("mirror rows after link = %d, want 2", n)
	}
	if _, err := capture(t, func() error {
		return cmdUnlink([]string{a, b, "--type", "blocks"})
	}); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if n := mirrorLinkRows(t, a, b); n != 0 {
		t.Fatalf("mirror rows after unlink = %d, want 0", n)
	}
}

// mirrorLinkRows counts the links rows between the two keys, both
// projections, through the same CLI surface a user would ask.
func mirrorLinkRows(t *testing.T, a, b string) int {
	t.Helper()
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header",
			"select i.key from links l join items it on it.id = l.item_id join issues i on i.item_id = it.id" +
				" where i.key in ('" + a + "','" + b + "')"})
	})
	if err != nil {
		t.Fatalf("sql: %v\n%s", err, out)
	}
	return len(strings.Fields(out))
}

// GDK-1816 — the verb that made it unmakes it. `gadak link KEY <url>` mints a
// remote link; before this round the only way back was `gadak ref KEY --list`
// then `--rm <id>`, a different verb and an id the person never typed.

// refListedIDs returns the remote-link ids on one issue, newest listing.
func refListedIDs(t *testing.T, key string) []string {
	t.Helper()
	out, err := capture(t, func() error { return cmdRef([]string{key, "--list", "--json"}) })
	if err != nil {
		t.Fatalf("ref --list: %v\n%s", err, out)
	}
	var listed struct {
		Refs []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"refs"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &listed); err != nil {
		t.Fatalf("ref --list json %q: %v", out, err)
	}
	ids := make([]string, 0, len(listed.Refs))
	for _, r := range listed.Refs {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestUnlinkURLRemovesWhatLinkURLCreated(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")
	url := "https://github.com/midagedev/gadak/pull/51"

	if out, err := capture(t, func() error { return cmdLink([]string{mine, url}) }); err != nil {
		t.Fatalf("link KEY url: %v\n%s", err, out)
	}
	if ids := refListedIDs(t, mine); len(ids) != 1 {
		t.Fatalf("after link, refs = %v, want one", ids)
	}

	if out, err := capture(t, func() error { return cmdUnlink([]string{mine, url}) }); err != nil {
		t.Fatalf("unlink KEY url: %v\n%s", err, out)
	}
	if ids := refListedIDs(t, mine); len(ids) != 0 {
		t.Fatalf("unlink KEY url left %v behind", ids)
	}
}

// The delete plan names the same id `ref --rm` would be given, so the two
// spellings of the removal are visibly one delete.
func TestUnlinkURLDryRunNamesTheSameIDAsRefRm(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")
	url := "https://example.com/design"

	if out, err := capture(t, func() error { return cmdLink([]string{mine, url, "--title", "design doc"}) }); err != nil {
		t.Fatalf("link KEY url: %v\n%s", err, out)
	}
	ids := refListedIDs(t, mine)
	if len(ids) != 1 {
		t.Fatalf("refs = %v, want one", ids)
	}

	plan := func(verb string, run func([]string) error, args []string) map[string]any {
		t.Helper()
		out, err := capture(t, func() error { return run(args) })
		if err != nil {
			t.Fatalf("%s --dry-run: %v\n%s", verb, err, out)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &body); err != nil {
			t.Fatalf("%s plan %q: %v", verb, out, err)
		}
		req, ok := body["request"].(map[string]any)
		if !ok {
			t.Fatalf("%s plan carries no request: %v", verb, body)
		}
		return req
	}

	byURL := plan("unlink", cmdUnlink, []string{mine, url, "--dry-run"})
	byID := plan("ref rm", cmdRef, []string{mine, "--rm", ids[0], "--dry-run"})
	if byURL["link_id"] != ids[0] || byID["link_id"] != ids[0] {
		t.Fatalf("the two removal spellings planned different ids: %v vs %v (listed %v)", byURL, byID, ids)
	}
	// The plan names what is going away, not just an opaque id.
	if byURL["url"] != url || byURL["title"] != "design doc" {
		t.Fatalf("the delete plan does not say what it removes: %v", byURL)
	}
	if got := refListedIDs(t, mine); len(got) != 1 {
		t.Fatalf("a dry run removed something: %v", got)
	}
}

func TestUnlinkURLNoMatchRemovesNothing(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")
	if out, err := capture(t, func() error {
		return cmdLink([]string{mine, "https://example.com/kept"})
	}); err != nil {
		t.Fatalf("link KEY url: %v\n%s", err, out)
	}

	_, err := capture(t, func() error { return cmdUnlink([]string{mine, "https://example.com/never"}) })
	if err == nil {
		t.Fatal("unlink of a URL this issue does not point at must be refused")
	}
	if !strings.Contains(err.Error(), "gadak ref "+mine+" --list") {
		t.Fatalf("the refusal must name the listing: %v", err)
	}
	if ids := refListedIDs(t, mine); len(ids) != 1 {
		t.Fatalf("the refused unlink changed the list: %v", ids)
	}
}

// A cross-workspace pointer is stored as a gadak:// URL, so it carries a
// scheme and reaches the same door — spelled as the pointer URL, not as the
// `<workspace>/<KEY>` shorthand `ref` accepts (that has no "://" and is
// `ref`'s grammar, not `link`'s).
func TestUnlinkURLRemovesACrossWorkspacePointer(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")
	if out, err := capture(t, func() error { return cmdRef([]string{mine, "elsewhere/ABC-1"}) }); err != nil {
		t.Fatalf("ref: %v\n%s", err, out)
	}
	if ids := refListedIDs(t, mine); len(ids) != 1 {
		t.Fatalf("after ref, refs = %v, want one", ids)
	}
	if out, err := capture(t, func() error {
		return cmdUnlink([]string{mine, "gadak://elsewhere/ABC-1"})
	}); err != nil {
		t.Fatalf("unlink KEY gadak://…: %v\n%s", err, out)
	}
	if ids := refListedIDs(t, mine); len(ids) != 0 {
		t.Fatalf("the pointer survived: %v", ids)
	}
}

// A remote link minted without a global id is a fresh row every time, so one
// URL can sit on an issue twice. The URL door must then refuse and hand the
// person back to the id, not silently delete whichever came first.
func TestUnlinkURLAmbiguousRefuses(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")
	url := "https://example.com/twice"
	for i := 0; i < 2; i++ {
		if out, err := capture(t, func() error { return cmdLink([]string{mine, url}) }); err != nil {
			t.Fatalf("link %d: %v\n%s", i, err, out)
		}
	}
	ids := refListedIDs(t, mine)
	if len(ids) != 2 {
		t.Skipf("this origin keeps one row per URL (%v) — no ambiguity to refuse", ids)
	}

	_, err := capture(t, func() error { return cmdUnlink([]string{mine, url}) })
	if err == nil {
		t.Fatal("two links point at that URL — unlink must refuse rather than pick one")
	}
	if !strings.Contains(err.Error(), "--rm <id>") {
		t.Fatalf("the refusal must hand the person the id spelling: %v", err)
	}
	if got := refListedIDs(t, mine); len(got) != 2 {
		t.Fatalf("the refused unlink deleted something: %v", got)
	}
}

// --type is the issue-link vocabulary; a URL target is a remote link. The
// mirror image of TestLinkURLWithTypeIsRefused.
func TestUnlinkURLWithTypeIsRefused(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")
	url := "https://example.com/design"
	if out, err := capture(t, func() error { return cmdLink([]string{mine, url}) }); err != nil {
		t.Fatalf("link KEY url: %v\n%s", err, out)
	}
	_, err := capture(t, func() error { return cmdUnlink([]string{mine, url, "--type", "blocks"}) })
	if err == nil {
		t.Fatal("--type with a URL target must be refused — the two link grammars do not mix")
	}
	if ids := refListedIDs(t, mine); len(ids) != 1 {
		t.Fatalf("the refused unlink removed something: %v", ids)
	}
}
