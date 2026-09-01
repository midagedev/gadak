package main

import (
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
//  4. local-origin: link then unlink through the in-process issuetap removes
//     both projections from the mirror — the cross-repo pin for the
//     synthetic id contract
//     TestUnlinkLocalOriginRemovesBothProjections

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

func TestUnlinkLocalOriginRemovesBothProjections(t *testing.T) {
	localOriginHome(t)
	a := createLocalOrigin(t, "unlink a")
	b := createLocalOrigin(t, "unlink b")
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
