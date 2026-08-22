//go:build livesmoke

package linear

// Lead-only live smoke (GDK-360/361): seeds demo issues into the real Linear
// workspace and exercises every write verb, including the attachment flow the
// adapter will need. Never committed to CI paths — guarded by the livesmoke
// tag and by requiring LINEAR_API_KEY in the environment.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("LINEAR_API_KEY")
	if key == "" {
		t.Skip("LINEAR_API_KEY not set")
	}
	c := New(key)
	c.HTTP = &http.Client{Timeout: 30 * time.Second}
	c.Retries = 2
	return c
}

func TestLiveSeedAndWriteVerbs(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	teams, err := c.Teams(ctx)
	if err != nil || len(teams) == 0 {
		t.Fatalf("teams: %v (%d)", err, len(teams))
	}
	team := teams[0]
	t.Logf("team: %s %s", team.ID, team.Key)

	states, err := c.WorkflowStates(ctx, team.ID)
	if err != nil || len(states) == 0 {
		t.Fatalf("states: %v", err)
	}
	byType := map[string]string{}
	for _, s := range states {
		if _, ok := byType[s.Type]; !ok {
			byType[s.Type] = s.ID
		}
	}
	t.Logf("state types: %v", byType)

	me, err := c.Viewer(ctx)
	if err != nil {
		t.Fatalf("viewer: %v", err)
	}

	seeds := []struct {
		title, desc string
		prio        int
		stateType   string
		due         string
	}{
		{"데모: 온보딩 플로우에서 토큰 필드 포커스가 풀린다", "재현: 첫 실행 → 토큰 페이지. 포커스가 body로 떨어진다.\n\n기대: 붙여넣기 필드에 커서.", 2, "backlog", ""},
		{"Demo: search returns stale rows after rename", "Steps: rename an issue, search old title — the row still shows.\nMirror watermark suspected.", 1, "unstarted", ""},
		{"데모: 다크 모드에서 코멘트 구분선이 안 보인다", "명암비 1.2:1 — 최소 3:1 필요.", 3, "backlog", ""},
		{"Demo: CSV export drops labels column", "Export any view with labels — column missing entirely.", 2, "started", ""},
		{"데모: 키보드 j/k가 그룹 헤더를 건너뛴다", "그룹 뷰에서 커서가 헤더 다음 행으로 점프.", 4, "backlog", ""},
		{"Demo: webhook retries hammer on 4xx", "4xx is permanent — retrying 20x per event.", 1, "unstarted", "2026-08-30"},
		{"데모: 첨부 미리보기가 5MB 이상에서 빈 화면", "PDF 6MB 업로드 후 미리보기 클릭 → 흰 화면.", 2, "started", ""},
		{"Demo: settings dialog steals Cmd+W", "Closes the window instead of the dialog.", 3, "backlog", ""},
		{"데모: 멘션 알림이 두 번 온다", "코멘트 수정 시 알림 재발송.", 2, "unstarted", ""},
		{"Demo: rate-limit header parsed as int overflow", "X-RateLimit-Reset epoch ms overflows int32 parse.", 1, "backlog", "2026-09-05"},
		{"데모: 보드 스크롤 위치가 새로고침마다 리셋", "긴 보드에서 새로고침 → 맨 위로.", 4, "backlog", ""},
		{"Demo: duplicate key on concurrent create", "Two tabs, same title, both succeed with same sort key.", 0, "unstarted", ""},
	}

	var created []Issue
	for i, s := range seeds {
		in := IssueCreate{TeamID: team.ID, Title: s.title, Description: s.desc, Priority: &s.prio}
		if id, ok := byType[s.stateType]; ok {
			in.StateID = id
		}
		if s.due != "" {
			in.DueDate = s.due
		}
		iss, err := c.CreateIssue(ctx, in)
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		created = append(created, iss)
		t.Logf("created %s %q", iss.Identifier, iss.Title)
	}

	// Update verbs on the first issue: retitle, state, priority, assign,
	// then unassign (the explicit-null case), label set replace.
	target := created[0]
	newTitle := target.Title + " (수정됨)"
	prio := 1
	assignee := me.ID
	upd, err := c.UpdateIssue(ctx, target.ID, IssueUpdate{
		Title: &newTitle, Priority: &prio, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatalf("update set: %v", err)
	}
	t.Logf("updated %s title/prio/assignee", upd.Identifier)
	unset := ""
	if _, err := c.UpdateIssue(ctx, target.ID, IssueUpdate{AssigneeID: &unset}); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	t.Log("unassigned (explicit null)")
	if start, ok := byType["started"]; ok {
		st := start
		if _, err := c.UpdateIssue(ctx, target.ID, IssueUpdate{StateID: &st}); err != nil {
			t.Fatalf("transition: %v", err)
		}
		t.Log("transitioned to started")
	}

	// Comments on a few.
	for i, body := range []string{
		"실측 재현 로그 첨부 예정 — cp949 콘솔에서만 발생.",
		"Confirmed on 0.16.0; regression from the watermark change.",
		"우선순위 상향: 데이터 유실 아님이 확인되기 전까지 High 유지.",
	} {
		if _, err := c.CreateComment(ctx, created[i].ID, body); err != nil {
			t.Fatalf("comment %d: %v", i, err)
		}
	}
	t.Log("3 comments created")

	// Attachment flow the GDK-361 Upload verb will use:
	// fileUpload → PUT to the signed URL → attachmentCreate(assetUrl).
	fileBody := []byte("gadak live smoke attachment — " + time.Now().Format(time.RFC3339) + "\n재현 로그 샘플.\n")
	up, err := c.UploadFile(ctx, "repro-log.txt", "text/plain", len(fileBody))
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, up.UploadURL, bytes.NewReader(fileBody))
	req.Header.Set("Content-Type", "text/plain")
	for k, v := range up.Headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT upload: %v", err)
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("PUT upload status %d", res.StatusCode)
	}
	att, err := c.CreateAttachment(ctx, created[6].ID, up.AssetURL, "repro-log.txt")
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}
	t.Logf("attachment %s on %s", att.ID, created[6].Identifier)

	fmt.Println("LIVESMOKE-OK", len(created), "issues")
}
