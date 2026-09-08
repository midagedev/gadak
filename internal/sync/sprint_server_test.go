package sync

import (
	"encoding/json"
	"testing"
)

// The Server wire shape, verbatim from Jira Software 11.3.11 (GDK-1650).
const serverSprintToString = `["com.atlassian.greenhopper.service.sprint.Sprint@4ffcc813[activatedDate=2026-09-08T14:15:49.589Z,autoStartStop=false,completeDate=<null>,endDate=2026-09-22T14:15:49.000Z,goal=<null>,id=1,incompleteIssuesDestinationId=<null>,name=Sprint 1,rapidViewId=1,sequence=1,startDate=2026-09-08T14:15:49.000Z,state=ACTIVE,synced=false]"]`

func TestPickSprintServerToString(t *testing.T) {
	id, name, state := pickSprint(json.RawMessage(serverSprintToString))
	if id == nil || *id != 1 {
		t.Fatalf("id = %v, want 1", id)
	}
	if name != "Sprint 1" {
		t.Fatalf("name = %q, want %q", name, "Sprint 1")
	}
	// ACTIVE on the wire; 'active' is what SKILL.md tells agents to query.
	if state != "active" {
		t.Fatalf("state = %q, want %q", state, "active")
	}
}

func TestPickSprintServerNameWithComma(t *testing.T) {
	raw := `["Sprint@1[goal=<null>,id=7,name=Q3, week 2,rapidViewId=1,state=CLOSED]"]`
	id, name, state := pickSprint(json.RawMessage(raw))
	if id == nil || *id != 7 || name != "Q3, week 2" || state != "closed" {
		t.Fatalf("got (%v, %q, %q)", id, name, state)
	}
}

func TestPickSprintServerPrefersActive(t *testing.T) {
	raw := `["S@1[id=4,name=old,rapidViewId=1,state=CLOSED]","S@2[id=9,name=now,rapidViewId=1,state=ACTIVE]"]`
	id, name, _ := pickSprint(json.RawMessage(raw))
	if id == nil || *id != 9 || name != "now" {
		t.Fatalf("got (%v, %q)", id, name)
	}
}

func TestPickSprintRejectsGarbageElement(t *testing.T) {
	if id, _, _ := pickSprint(json.RawMessage(`[42]`)); id != nil {
		t.Fatalf("a number element must empty the result, got %v", id)
	}
}
