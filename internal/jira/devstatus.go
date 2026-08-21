package jira

// The development-panel read (GDK-496/497). On Jira Cloud this is the
// internal /rest/dev-status API — Atlassian's own workaround for the panel
// having no public read (JSWCLOUD-16901): works with the site API token,
// explicitly unsupported and free to change. issuetap serves the same two
// GETs on purpose, so this one reader covers connected and standalone.
// Callers gate it behind config.DevStatus and treat every error as
// "skip this issue", never as a sync failure.

import (
	"context"
	"fmt"
	"strings"
)

// DevPRStatus is the origin's development-panel pull-request state
// (Jira Cloud / issuetap). These three tokens are the whole vocabulary.
type DevPRStatus string

const (
	DevPROpen     DevPRStatus = "OPEN"
	DevPRMerged   DevPRStatus = "MERGED"
	DevPRDeclined DevPRStatus = "DECLINED"
)

// ParseDevPRStatus accepts the origin/CLI tokens (any case). Unknown
// input is rejected — callers that need a default (gh scan) use
// DevPRStatusFromGitHub.
func ParseDevPRStatus(s string) (DevPRStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case string(DevPROpen):
		return DevPROpen, true
	case string(DevPRMerged):
		return DevPRMerged, true
	case string(DevPRDeclined):
		return DevPRDeclined, true
	default:
		return "", false
	}
}

// DevPRStatusFromGitHub maps `gh pr list --json state` onto the origin
// vocabulary. CLOSED is DECLINED; anything else (including OPEN) is OPEN.
func DevPRStatusFromGitHub(ghState string) DevPRStatus {
	switch strings.ToUpper(strings.TrimSpace(ghState)) {
	case string(DevPRMerged):
		return DevPRMerged
	case "CLOSED":
		return DevPRDeclined
	default:
		return DevPROpen
	}
}

// Stored is the mirror-column form (lowercase). store.DevLink.Status
// stores this so list/detail JSON stays "open"|"merged"|"declined".
func (s DevPRStatus) Stored() string {
	return strings.ToLower(string(s))
}

// DevBuildState is the build-record state vocabulary (GDK-592): the three
// buckets the dev-status summary counts. Deployment states are free-form —
// only "successful" is load-bearing there — so they stay plain strings.
type DevBuildState string

const (
	DevBuildSuccessful DevBuildState = "successful"
	DevBuildFailed     DevBuildState = "failed"
	DevBuildUnknown    DevBuildState = "unknown"
)

// ParseDevBuildState accepts the CLI/origin tokens (any case); anything
// outside the three buckets is rejected — an in-progress build is not a
// state this vocabulary has.
func ParseDevBuildState(s string) (DevBuildState, bool) {
	switch st := DevBuildState(strings.ToLower(strings.TrimSpace(s))); st {
	case DevBuildSuccessful, DevBuildFailed, DevBuildUnknown:
		return st, true
	default:
		return "", false
	}
}

// DevDeployment is one deployment record the link POST answers (GDK-592).
// issuetap's own vocabulary — Cloud's detail rows for deployments were
// never captured, so no read path produces this type; it exists for the
// write's 201 echo. A url-less deployment's ID is environment:<name>.
type DevDeployment struct {
	ID          string     `json:"id"`
	URL         string     `json:"url"`
	Environment string     `json:"environment"`
	State       string     `json:"state"`
	Actor       DevPRActor `json:"actor"`
}

// DevBuild is one build record the link POST answers (GDK-592). Number is
// the build number as a string; a url-less build's ID is build:<number>.
type DevBuild struct {
	ID     string     `json:"id"`
	URL    string     `json:"url"`
	Number string     `json:"number"`
	State  string     `json:"state"`
	Actor  DevPRActor `json:"actor"`
}

// DevPRAuthor is the PR author block (Cloud vocabulary: author.name is the
// GitHub login).
type DevPRAuthor struct {
	Name string `json:"name"`
}

// DevPRSource is where the PR heads from (Cloud vocabulary: source.branch is
// the head ref).
type DevPRSource struct {
	Branch string `json:"branch"`
}

// DevPRActor is issuetap's extension naming who wrote the link: accountId is
// the X-Issuetap-Actor slug, displayName its human form. Cloud's dev-status
// has no such block — both fields stay empty there.
type DevPRActor struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

// DevPR is one pull request from the dev-status detail payload.
type DevPR struct {
	ID     string      `json:"id"`
	URL    string      `json:"url"`
	Name   string      `json:"name"`
	Status DevPRStatus `json:"status"`
	// Author is the pull request's author. Actor is who WROTE the link —
	// issuetap stamps it from the request identity and serves it back;
	// Cloud has no such field and it stays empty. Different axes — a bot
	// links a human's PR — never merged (GDK-589). Source is the head ref.
	Author DevPRAuthor `json:"author"`
	Source DevPRSource `json:"source"`
	Actor  DevPRActor  `json:"actor"`
}

// DevStatusPRCount reads the summary's pullrequest count — one cheap call to
// decide whether the detail call is worth making. issueID is the origin's
// issue id verbatim (numeric on Cloud), never the key.
func (c *Client) DevStatusPRCount(ctx context.Context, issueID string) (int, error) {
	var out struct {
		Summary struct {
			PullRequest struct {
				Overall struct {
					Count int `json:"count"`
				} `json:"overall"`
			} `json:"pullrequest"`
		} `json:"summary"`
	}
	p := fmt.Sprintf("/rest/dev-status/latest/issue/summary?issueId=%s", issueID)
	if err := c.do(ctx, "GET", p, nil, &out); err != nil {
		return 0, err
	}
	return out.Summary.PullRequest.Overall.Count, nil
}

// DevStatusPRs reads the pull requests the panel holds for one issue.
// applicationType=GitHub is what Cloud's own frontend sends for the GitHub
// app (and issuetap accepts any value); the response is parsed defensively —
// unknown fields are ignored, missing ones stay zero.
func (c *Client) DevStatusPRs(ctx context.Context, issueID string) ([]DevPR, error) {
	var out struct {
		Detail []struct {
			PullRequests []DevPR `json:"pullRequests"`
		} `json:"detail"`
	}
	p := fmt.Sprintf("/rest/dev-status/1.0/issue/detail?issueId=%s&applicationType=GitHub&dataType=pullrequest", issueID)
	if err := c.do(ctx, "GET", p, nil, &out); err != nil {
		return nil, err
	}
	var prs []DevPR
	for _, d := range out.Detail {
		for _, pr := range d.PullRequests {
			if st, ok := ParseDevPRStatus(string(pr.Status)); ok {
				pr.Status = st
			} else {
				pr.Status = DevPRStatus(strings.ToUpper(string(pr.Status)))
			}
			prs = append(prs, pr)
		}
	}
	return prs, nil
}

// LinkDevPR posts one pull-request link to the origin's dev-status store.
// Only issuetap (standalone) implements the endpoint — Jira Cloud's panel is
// written by its marketplace apps, so a connected origin answers 404 and the
// caller says so instead of pretending. author (the PR author's login) and
// branch (the head ref) are optional (GDK-589): empty ones are omitted so a
// re-link without them keeps what the origin already holds, and an older
// origin ignores both keys. The actor is never sent — issuetap stamps it
// from the request identity.
func (c *Client) LinkDevPR(ctx context.Context, issueID, prURL, name, author, branch string, status DevPRStatus) (DevPR, error) {
	var out DevPR
	body := map[string]any{"issueId": issueID, "url": prURL, "name": name, "status": string(status)}
	if author != "" {
		body["author"] = author
	}
	if branch != "" {
		body["branch"] = branch
	}
	err := c.write(ctx, "POST", "/rest/dev-status/1.0/issue/link", body, &out)
	return out, err
}

// LinkDevDeployment posts one deployment record (GDK-592). environment
// and state are required by the origin; url is optional — omitted from
// the body when empty so the origin keys the row by its environment. The
// actor is never sent — issuetap stamps it from the request identity.
func (c *Client) LinkDevDeployment(ctx context.Context, issueID, environment, state, url string) (DevDeployment, error) {
	var out DevDeployment
	body := map[string]any{"issueId": issueID, "kind": "deployment", "environment": environment, "state": state}
	if url != "" {
		body["url"] = url
	}
	err := c.write(ctx, "POST", "/rest/dev-status/1.0/issue/link", body, &out)
	return out, err
}

// LinkDevBuild posts one build record (GDK-592). state is the closed
// three-bucket vocabulary; url and number are optional keys (the origin
// requires one) and are omitted from the body when empty.
func (c *Client) LinkDevBuild(ctx context.Context, issueID string, state DevBuildState, number, url string) (DevBuild, error) {
	var out DevBuild
	body := map[string]any{"issueId": issueID, "kind": "build", "state": string(state)}
	if number != "" {
		body["number"] = number
	}
	if url != "" {
		body["url"] = url
	}
	err := c.write(ctx, "POST", "/rest/dev-status/1.0/issue/link", body, &out)
	return out, err
}
