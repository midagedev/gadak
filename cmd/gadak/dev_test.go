package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/store"
)

// GDK-531: dev scan's pure parts — key extraction from PR titles/branches
// and gh's state vocabulary mapped onto dev-status's.
func TestIssueKeys(t *testing.T) {
	got := issueKeys("GDK-531 fix the drop (gdk-531-scan-branch) and STD-2, again GDK-531")
	want := []string{"GDK-531", "STD-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("issueKeys = %v, want %v", got, want)
	}
	if got := issueKeys("no keys here, just v1.2-3 and 12-34"); got != nil {
		t.Fatalf("issueKeys on keyless text = %v", got)
	}
	// Key-shaped garbage (CVE-2024-1234) still matches — the mirror filter
	// owns rejecting it, so the extractor stays dumb on purpose.
	if got := issueKeys("fixes CVE-2024-1234"); len(got) == 0 {
		t.Fatal("extractor should stay broad; the mirror filters")
	}
}

// GDK-1103: the scan body has a single owner — this pattern must be assembled
// from fields.IssueKeyBare so a charset edit here cannot drift silently from
// internal/store's copy (a re-inlined body that diverges stops containing the
// owner fragment).
func TestIssueKeyScanBodyHasSingleOwner(t *testing.T) {
	if !strings.Contains(issueKeyRe.String(), fields.IssueKeyBare) {
		t.Fatalf("issueKeyRe = %q, want it assembled from fields.IssueKeyBare (%q)",
			issueKeyRe.String(), fields.IssueKeyBare)
	}
}

func TestDevScanStatus(t *testing.T) {
	for in, want := range map[string]jira.DevPRStatus{
		"OPEN": jira.DevPROpen, "open": jira.DevPROpen, "MERGED": jira.DevPRMerged,
		"CLOSED": jira.DevPRDeclined, "closed": jira.DevPRDeclined, "weird": jira.DevPROpen,
	} {
		if got := devScanStatus(in); got != want {
			t.Fatalf("devScanStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDevOriginWriteGate(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")

	localOrigin := &config.Config{Kind: config.KindLocalOrigin}
	if err := refuseConnectedDevWrite(localOrigin, "dev link"); err != nil {
		t.Fatalf("local-origin refused: %v", err)
	}

	connected := &config.Config{Site: "https://example.atlassian.net", Email: "a@b.c", Token: "t"}
	err := refuseConnectedDevWrite(connected, "dev link")
	if err == nil {
		t.Fatal("plain connected must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gadak config set devStatus true") {
		t.Errorf("connected refusal must name config set: %q", msg)
	}
	if !strings.Contains(msg, "GitHub app") {
		t.Errorf("connected refusal must name Jira's GitHub app: %q", msg)
	}
	if strings.Contains(msg, "config.json") {
		t.Errorf("must not tell the user to hand-edit config.json: %q", msg)
	}

	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.SaveRemote(dir, pairing.Remote{
		Endpoint: "https://home.example:8443",
		Token:    "device-token",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	paired := &config.Config{}
	if err := refuseConnectedDevWrite(paired, "dev scan"); err != nil {
		t.Fatalf("paired refused: %v", err)
	}
}

func TestDevScanEmptyClassifier(t *testing.T) {
	if got := devScanNoMatchMessage(0); got != "dev scan: no pull requests" {
		t.Fatalf("0 PRs: %q", got)
	}
	if got := devScanNoMatchMessage(3); !strings.Contains(got, "mention") {
		t.Fatalf("PRs without keys: %q", got)
	}
	if devScanNoMatchMessage(0) == devScanNoMatchMessage(3) {
		t.Fatal("0 PRs and keyless PRs must be different sentences")
	}
}

func TestDevScanLimitNotice(t *testing.T) {
	if got := devScanHitLimitNotice(200, 200); got != "first 200 PRs scanned — raise --limit" {
		t.Fatalf("at cap: %q", got)
	}
	if got := devScanHitLimitNotice(199, 200); got != "" {
		t.Fatalf("under cap must be silent, got %q", got)
	}
}

func TestInstallDevScanHookWritesWorkspaceAndKeepsStderr(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if out, err := exec.Command("git", "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("laptop")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Kind: config.KindLocalOrigin}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := installDevScanHook(); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(root, ".git", "hooks", "pre-push")
	raw, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `gadak --workspace "laptop" dev scan`) &&
		!strings.Contains(body, `gadak --workspace laptop dev scan`) {
		t.Fatalf("hook missing workspace-qualified scan: %s", body)
	}
	if strings.Contains(body, "2>&1") {
		t.Fatalf("hook still swallows stderr: %s", body)
	}
	if strings.Contains(body, "|| true") {
		t.Fatalf("hook still uses || true: %s", body)
	}
	if !strings.Contains(body, "gadak dev scan failed (push continues)") {
		t.Fatalf("hook missing visible failure line: %s", body)
	}
}

func TestInstallDevScanHookRefusesConnected(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	err := cmdDevScan([]string{"--install-hook"})
	if err == nil {
		t.Fatal("connected --install-hook must be refused")
	}
	if !strings.Contains(err.Error(), "gadak config set devStatus true") {
		t.Fatalf("refusal %q", err)
	}
}

// GDK-589: author/branch render as trailing tab columns only when present,
// so old origins and keyless PRs keep the exact pre-589 line.
func TestDevScanMatchExtras(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"", ""}:                   "",
		{"midagedev", ""}:          "\tmidagedev",
		{"", "gdk-589-x"}:          "\tgdk-589-x",
		{"midagedev", "gdk-589-x"}: "\tmidagedev\tgdk-589-x",
	} {
		if got := devScanMatchExtras(in[0], in[1]); got != want {
			t.Errorf("devScanMatchExtras(%q,%q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestDevLinkExtrasJSON(t *testing.T) {
	if got := devLinkExtrasJSON("", ""); got != "" {
		t.Errorf("empty = %q, want no members", got)
	}
	if got := devLinkExtrasJSON("midagedev", "gdk-589-x"); got != `,"author":"midagedev","branch":"gdk-589-x"` {
		t.Errorf("both = %q", got)
	}
	if got := devLinkExtrasJSON("", "gdk-589-x"); got != `,"branch":"gdk-589-x"` {
		t.Errorf("branch only = %q", got)
	}
}

func TestCurrentGitBranch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if out, err := exec.Command("git", "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// CI has no git identity, so the commit carries its own (GDK-589).
	if out, err := exec.Command("git",
		"-c", "user.name=gadak test", "-c", "user.email=test@example.invalid",
		"commit", "--allow-empty", "-m", "x").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "checkout", "-b", "gdk-589-dev-link-actor").CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b: %v\n%s", err, out)
	}
	if got := currentGitBranch(); got != "gdk-589-dev-link-actor" {
		t.Fatalf("currentGitBranch = %q, want gdk-589-dev-link-actor", got)
	}
	// Detached HEAD (git's literal "HEAD") must read as no branch, not as a
	// ref named HEAD.
	if out, err := exec.Command("git", "checkout", "--detach").CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach: %v\n%s", err, out)
	}
	if got := currentGitBranch(); got != "" {
		t.Fatalf("detached currentGitBranch = %q, want empty", got)
	}
}

// TestParseDevScanPRs pins gh's --json author shape (GDK-589): the login
// rides inside an author object, JSON null (deleted account) reads as an
// empty login, and headRefName survives as the link's branch.
func TestParseDevScanPRs(t *testing.T) {
	prs, err := parseDevScanPRs([]byte(`[
	  {"url":"https://github.com/o/r/pull/9","title":"GDK-589 dev links carry author",
	   "state":"OPEN","headRefName":"gdk-589-dev-links",
	   "author":{"login":"midagedev","name":"Mid Agedev","id":"MDQ6VXNlcjE="}},
	  {"url":"https://github.com/o/r/pull/10","title":"GDK-354 pin darwin","state":"MERGED",
	   "headRefName":"gdk-354-darwin","author":null}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}
	if prs[0].Author.Login != "midagedev" || prs[0].HeadRefName != "gdk-589-dev-links" {
		t.Errorf("prs[0] author/head = %q/%q, want midagedev / gdk-589-dev-links",
			prs[0].Author.Login, prs[0].HeadRefName)
	}
	if prs[1].Author.Login != "" || prs[1].HeadRefName != "gdk-354-darwin" {
		t.Errorf("prs[1] author/head = %q/%q, want empty / gdk-354-darwin",
			prs[1].Author.Login, prs[1].HeadRefName)
	}
}

// GDK-592: the origin's deployment/build rows map onto dev_links without
// stuffing kind data into display columns — environment gets its own
// column, the build number rides external_id, and a url-less row falls
// back to the origin's id as the mirror's idempotent key.
func TestDevLinkRowsFromDeploymentAndBuild(t *testing.T) {
	dep := devLinkFromDeployment(jira.DevDeployment{
		ID: "environment:production", Environment: "production", State: "successful",
	})
	if dep.Kind != "deployment" || dep.Environment != "production" || dep.Status != "successful" {
		t.Fatalf("deployment row = %+v", dep)
	}
	if dep.URL != "environment:production" || dep.ExternalID != "environment:production" {
		t.Fatalf("url-less deployment key = %q/%q, want the origin id on both", dep.URL, dep.ExternalID)
	}
	withURL := devLinkFromDeployment(jira.DevDeployment{
		ID: "https://ci/run/9", URL: "https://ci/run/9", Environment: "staging", State: "failed",
	})
	if withURL.URL != "https://ci/run/9" || withURL.Environment != "staging" {
		t.Fatalf("url-keyed deployment row = %+v", withURL)
	}

	b := devLinkFromBuild(jira.DevBuild{
		ID: "https://ci.example/gadak/build/592", URL: "https://ci.example/gadak/build/592",
		Number: "592", State: "failed",
	})
	if b.Kind != "build" || b.ExternalID != "592" || b.Status != "failed" || b.Environment != "" {
		t.Fatalf("build row = %+v", b)
	}
}

// TestIssuePrintsDeploymentAndBuildLinks pins `gadak issue`'s display:
// deployments and builds render in their own kind-labelled sections, not
// inside Linked PRs (which stays pull requests only).
func TestIssuePrintsDeploymentAndBuildLinks(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	ctx := context.Background()
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.ReplaceDevLinks(ctx, "NMB-1", store.DevLinksUpdate{Links: []store.DevLink{
		{Kind: "pullrequest", URL: "https://github.com/o/r/pull/9", Title: "fix the drop", Status: "merged"},
		{Kind: "deployment", ExternalID: "environment:production", URL: "environment:production",
			Environment: "production", Status: "successful", UpdatedAt: "2026-08-22T00:00:00.000Z"},
		{Kind: "build", ExternalID: "592", URL: "https://ci.example/gadak/build/592",
			Status: "failed", UpdatedAt: "2026-08-22T00:00:00.000Z"},
	}}); err != nil {
		t.Fatal(err)
	}
	lites, err := db.IssueLitesByKeys(ctx, []string{"NMB-1"})
	if err != nil || len(lites) == 0 {
		t.Fatalf("lites: %v (%d)", err, len(lites))
	}
	d, err := db.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := capture(t, func() error { printIssue(lites[0], d, store.Spans{}); return nil })
	for _, want := range []string{
		"Linked PRs (1)",
		"https://github.com/o/r/pull/9",
		"deployments (1)",
		"production\tsuccessful",
		"builds (1)",
		"#592\tfailed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("issue output missing %q\n%s", want, out)
		}
	}
	if strings.Count(out, "https://ci.example/gadak/build/592") < 1 {
		t.Errorf("build url not shown:\n%s", out)
	}
}
