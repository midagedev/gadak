package origin

import (
	"context"
	"testing"
)

// A decorator embeds Writer, which promotes only what Writer declares. Every
// As* answer went false the moment the actor trailer wrapped a jiraWriter
// (GDK-1655) — this pins the unwrap that fixed it.
type fakeSprintWriter struct{ Writer }

func (fakeSprintWriter) MoveToSprint(context.Context, int64, []string) error { return nil }
func (fakeSprintWriter) MoveToBacklog(context.Context, []string) error       { return nil }
func (fakeSprintWriter) CreateSprint(context.Context, int64, string, string) (Sprint, error) {
	return Sprint{}, nil
}
func (fakeSprintWriter) UpdateSprint(context.Context, int64, map[string]any) (Sprint, error) {
	return Sprint{}, nil
}

func TestCapabilitySeesThroughTheTrailerWrapper(t *testing.T) {
	inner := fakeSprintWriter{}
	if _, err := AsSprintBoard(inner); err != nil {
		t.Fatalf("bare writer: %v", err)
	}
	wrapped := &actorTrailerWriter{Writer: inner, trailer: "— by a test"}
	if _, err := AsSprintBoard(wrapped); err != nil {
		t.Fatalf("wrapped writer: %v — the capability disappeared behind the decorator", err)
	}
	jiraShaped := &jiraActorTrailerWriter{actorTrailerWriter{Writer: inner, trailer: "— by a test"}}
	if _, err := AsSprintBoard(jiraShaped); err != nil {
		t.Fatalf("jira-shaped wrapper: %v", err)
	}
}

func TestCapabilityRefusesAnOriginWithoutIt(t *testing.T) {
	var plain Writer = stubWriter{}
	if _, err := AsSprintBoard(plain); err == nil {
		t.Fatal("an origin with no sprints must be refused by name, not silently succeed")
	}
}

type stubWriter struct{ Writer }
