// Package claim is the single owner of the gadak claim write (CLI today,
// REST later), the way internal/transition owns the transition write.
//
// On an origin that implements issuetap's claim route — standalone and
// paired workspaces — Apply is one atomic call: of two agents claiming
// concurrently exactly one wins. Atlassian Cloud has no claim route and
// answers 404; there Apply falls back to the two writes the atomic route
// exists to fuse (assignee + in-progress transition), judged locally first.
// That fallback has no atomicity; Result.Atomic says which path answered so
// the caller can say so instead of silently granting a guarantee the origin
// never made.
package claim

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/statuscat"
	"github.com/midagedev/gadak/internal/transition"
)

// categoryInProgress is the mirror's status-category token for "in progress".
// PickTransition accepts it as a transition target; every category token is
// the mirror's new|inprogress|done, never the origin's indeterminate.
const categoryInProgress = "inprogress"

// Origin is the origin surface Apply needs. *jira.Client satisfies it; the
// Linear adapter does not — claim is a Jira-workflow verb (assignee plus the
// in-progress transition), so callers refuse Linear before reaching here.
type Origin interface {
	transition.Origin
	Claim(ctx context.Context, key, transitionID string, takeOver bool) (jira.ClaimResult, error)
	Myself(ctx context.Context) (jira.User, error)
	IssueStatus(ctx context.Context, key string) (jira.Status, *jira.User, error)
	SetAssignee(ctx context.Context, key, accountID string) error
}

// Request is one claim write. TransitionID is optional: empty lets the
// origin pick the first destination whose category is in-progress. Set, it
// accepts whatever PickTransition accepts (transition id, status id, name)
// and is the answer to a board where two transitions land in progress
// (GDK-1174) — Apply resolves it and refuses a destination that is not
// in-progress, because claim is not a general transition.
type Request struct {
	Key          string
	TransitionID string
	TakeOver     bool
}

// Result is what the caller shows: the claim's outcome on the origin plus
// whether the origin guaranteed it atomically.
type Result struct {
	Key            string `json:"key"`
	AssigneeID     string `json:"assignee_id"`
	Assignee       string `json:"assignee"`
	StatusID       string `json:"status_id"`
	Status         string `json:"status"`
	StatusCategory string `json:"status_category"`
	ClaimedAt      string `json:"claimed_at"`
	Atomic         bool   `json:"atomic"`
}

// TakenError is the refusal: another actor holds the issue in progress. The
// sentence stays the origin's own ("<KEY> is already claimed by <표시명>")
// on both paths so they print identically; Holder carries the display name
// separately for callers that show it on its own. Exit codes and HTTP
// status are the caller's mapping (CLI: a dedicated exit code; REST: 409).
type TakenError struct {
	Key    string
	Holder string
	msg    string
}

func (e *TakenError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	holder := e.Holder
	if holder == "" {
		holder = "someone else"
	}
	return fmt.Sprintf("%s is already claimed by %s", e.Key, holder)
}

// Apply claims req.Key on the origin. Standalone and paired origins answer
// the atomic route; a 404 means the origin has no claim route (Cloud — or
// the issue does not exist, which the fallback's read then reports honestly)
// and the two-call fallback runs. Apply does not refresh the mirror — that
// tail is the write verb's, same as transition.
func Apply(ctx context.Context, o Origin, cfg *config.Config, req Request) (Result, error) {
	if req.TransitionID != "" {
		id, err := resolveExplicitTransition(ctx, o, req.Key, req.TransitionID)
		if err != nil {
			return Result{}, err
		}
		req.TransitionID = id
	}
	res, err := o.Claim(ctx, req.Key, req.TransitionID, req.TakeOver)
	if err == nil {
		return fromOrigin(res, true), nil
	}
	var api *jira.APIError
	if !errors.As(err, &api) {
		return Result{}, err
	}
	switch api.Status {
	case http.StatusConflict:
		e := &TakenError{Key: req.Key, msg: api.Message()}
		if rest, ok := strings.CutPrefix(e.msg, req.Key+" is already claimed by "); ok {
			e.Holder = rest
		}
		return Result{}, e
	case http.StatusNotFound:
		return cloudFallback(ctx, o, cfg, req)
	}
	return Result{}, err
}

// cloudFallback is the connected-Cloud path: read the two facts the atomic
// route would judge, judge them the same way issuetap does, then write the
// two calls that route fuses. In progress and held by another account is a
// refusal unless TakeOver; already mine is a no-op; otherwise transition
// (only when not already in progress), then assign — transition first, so a
// transition the origin rejects leaves the assignee untouched.
func cloudFallback(ctx context.Context, o Origin, cfg *config.Config, req Request) (Result, error) {
	me, err := o.Myself(ctx)
	if err != nil {
		return Result{}, err
	}
	st, holder, err := o.IssueStatus(ctx, req.Key)
	if err != nil {
		return Result{}, err
	}
	inProgress := statuscat.Category(st.StatusCategory.Key) == categoryInProgress
	if inProgress && holder != nil && holder.AccountID != "" && holder.AccountID != me.AccountID && !req.TakeOver {
		name := holder.DisplayName
		if name == "" {
			name = holder.AccountID
		}
		return Result{}, &TakenError{Key: req.Key, Holder: name}
	}
	if !inProgress {
		target := categoryInProgress
		if req.TransitionID != "" {
			target = req.TransitionID
		}
		if _, err := transition.Apply(ctx, o, cfg, transition.Request{Key: req.Key, Target: target}); err != nil {
			return Result{}, err
		}
	}
	if holder == nil || holder.AccountID != me.AccountID {
		if err := o.SetAssignee(ctx, req.Key, me.AccountID); err != nil {
			return Result{}, err
		}
	}
	// Re-read: the two writes may have moved either field, and the result
	// line should say what is true now, not what the fallback assumed.
	st, who, err := o.IssueStatus(ctx, req.Key)
	if err != nil {
		return Result{}, err
	}
	who = coalesceUser(who, me)
	return Result{
		Key:            req.Key,
		AssigneeID:     who.AccountID,
		Assignee:       who.DisplayName,
		StatusID:       st.ID,
		Status:         st.Name,
		StatusCategory: statuscat.Category(st.StatusCategory.Key),
		// No origin stamp exists for "when the claim happened" here —
		// claimed_at is the atomic route's answer and stays empty on it.
		Atomic: false,
	}, nil
}

// resolveExplicitTransition turns whatever the caller passed (transition id,
// status id, name) into the one transition id both write paths take, and
// refuses a destination outside the in-progress category — claim moves an
// issue into progress; anything else is `gadak transition`.
func resolveExplicitTransition(ctx context.Context, o Origin, key, want string) (string, error) {
	list, err := o.Transitions(ctx, key)
	if err != nil {
		return "", err
	}
	id, err := jira.PickTransition(key, want, list)
	if err != nil {
		return "", err
	}
	for _, t := range list {
		if t.ID != id {
			continue
		}
		if cat, ok := statuscat.KnownCategory(t.To.StatusCategory.Key); !ok || cat != categoryInProgress {
			return "", fmt.Errorf("claim takes an issue into progress — %s lands on %s; use gadak transition for that move",
				jira.FormatTransition(t), t.To.Name)
		}
	}
	return id, nil
}

func fromOrigin(res jira.ClaimResult, atomic bool) Result {
	return Result{
		Key:            res.Key,
		AssigneeID:     res.Assignee.AccountID,
		Assignee:       res.Assignee.DisplayName,
		StatusID:       res.Status.ID,
		Status:         res.Status.Name,
		StatusCategory: statuscat.Category(res.Status.StatusCategory.Key),
		ClaimedAt:      res.ClaimedAt,
		Atomic:         atomic,
	}
}

// coalesceUser keeps the fallback's result honest when the re-read comes
// back without an assignee (sites that drop it on transition): the write
// above set it to me, so me is what the answer says.
func coalesceUser(who *jira.User, me jira.User) *jira.User {
	if who != nil && who.AccountID != "" {
		return who
	}
	return &me
}
