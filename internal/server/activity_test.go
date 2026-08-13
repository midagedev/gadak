package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// activityProgress is the GET sync/progress/ body including the activity field.
// Existing progressDoc fields keep their meaning; activity is additive.
type activityProgress struct {
	progressDoc
	Activity mirrorActivity `json:"activity"`
}

func resetJobAndActivity(t *testing.T) *Handler {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Projects = nil
	// New server instance starts with zero job/activity; no package-global reset.
	return New(db, cfg)
}

func getProgressActivity(t *testing.T, h http.Handler) activityProgress {
	t.Helper()
	rec := get(t, h, apiBase+"sync/progress/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	var p activityProgress
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode progress: %v\nbody: %s", err, rec.Body.String())
	}
	return p
}

// TestSyncActivityHooksOnProgress: phase("documents") + progress(50,50) lands
// on GET sync/progress/ as activity with that source and count.
func TestSyncActivityHooksOnProgress(t *testing.T) {
	h := resetJobAndActivity(t)
	phase, progress := h.SyncActivityHooks()
	if phase == nil || progress == nil {
		t.Fatal("SyncActivityHooks returned nil funcs")
	}
	phase(syncPhaseDocuments())
	progress(50, 50)

	p := getProgressActivity(t, h)
	if !p.Activity.Running {
		t.Fatalf("activity.running = false, want true; %+v", p.Activity)
	}
	if p.Activity.Source != "documents" {
		t.Fatalf("activity.source = %q, want documents", p.Activity.Source)
	}
	if p.Activity.Fetched != 50 {
		t.Fatalf("activity.fetched = %d, want 50", p.Activity.Fetched)
	}
	if p.Activity.Changed != 50 {
		t.Fatalf("activity.changed = %d, want 50", p.Activity.Changed)
	}
	if p.Activity.StartedAt == "" {
		t.Fatal("activity.started_at empty")
	}
}

// TestSyncActivitySourceChangeResetsCounters: switching source is a new pass;
// previous totals must not carry over (issues 6932 → documents starts at 0).
func TestSyncActivitySourceChangeResetsCounters(t *testing.T) {
	h := resetJobAndActivity(t)
	phase, progress := h.SyncActivityHooks()
	phase("issues")
	progress(6932, 100)
	p := getProgressActivity(t, h)
	if p.Activity.Fetched != 6932 || p.Activity.Source != "issues" {
		t.Fatalf("after issues progress: %+v", p.Activity)
	}

	phase("documents")
	p = getProgressActivity(t, h)
	if p.Activity.Source != "documents" {
		t.Fatalf("source after switch: %q", p.Activity.Source)
	}
	if p.Activity.Fetched != 0 || p.Activity.Changed != 0 {
		t.Fatalf("counters must reset on source change: %+v", p.Activity)
	}
	if !p.Activity.Running {
		t.Fatal("activity should still be running after source switch")
	}
}

// TestSyncActivityPhaseIdleCloses: phase("") clears the activity slot.
func TestSyncActivityPhaseIdleCloses(t *testing.T) {
	h := resetJobAndActivity(t)
	phase, progress := h.SyncActivityHooks()
	phase("documents")
	progress(10, 10)
	phase("")

	p := getProgressActivity(t, h)
	if p.Activity.Running {
		t.Fatalf("activity.running still true after idle: %+v", p.Activity)
	}
	if p.Activity.Source != "" {
		t.Fatalf("activity.source = %q, want empty", p.Activity.Source)
	}
	if p.Activity.Fetched != 0 || p.Activity.Changed != 0 {
		t.Fatalf("counters should clear on idle: %+v", p.Activity)
	}
}

// TestSyncActivityDoesNotImplyJobRunning: the one-shot job's `running` field is
// independent of background activity. POST sync/ → 409 still means the job,
// not "something is fetching".
func TestSyncActivityDoesNotImplyJobRunning(t *testing.T) {
	h := resetJobAndActivity(t)
	phase, _ := h.SyncActivityHooks()
	phase("documents")

	p := getProgressActivity(t, h)
	if p.Activity.Running != true {
		t.Fatalf("activity should be running: %+v", p.Activity)
	}
	// Job lifecycle fields must stay idle — client start/poll depends on this.
	if p.Running {
		t.Fatalf("job running = true while only activity is on; progressDoc %+v", p.progressDoc)
	}
	if p.Phase != "idle" {
		t.Fatalf("job phase = %q, want idle", p.Phase)
	}
	if p.Done {
		t.Fatal("job done should be false when idle")
	}

	// No credential → POST still 400 credential_required, not 409 from a phantom job.
	rec := send(t, h, http.MethodPost, apiBase+"sync/", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST sync/ without credential: %d %s (activity must not start a job)", rec.Code, rec.Body.String())
	}
}

// syncPhaseDocuments is the documents source string. Tests use the literal the
// API promises ("documents") so a rename of the sync package constant is visible.
func syncPhaseDocuments() string { return "documents" }
