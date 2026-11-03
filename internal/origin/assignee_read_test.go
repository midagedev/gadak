package origin

// The assign preflight read (GDK-2063): every writer that can carry an
// assignee write must also answer "who holds this issue right now" — the
// read the CLI's stale check compares with the mirror's issues.assignee_id.
//
//	R1 Linear answers from one issue(id:) fetch, assignee and all
//	   TestLinearWriterIssueAssignee (FAIL-first: pre-change linearWriter
//	   had no IssueAssignee — the rows cannot compile against it)
//	R2 the actor trailer forwards the face, so an assign on a workspace
//	   with a resolved actor still reads instead of refusing
//	   TestActorTrailerForwardsIssueAssignee (FAIL-first: interface
//	   embedding alone drops the face — the hit row fails without the
//	   forwarding method)

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
)

func TestLinearWriterIssueAssignee(t *testing.T) {
	rec := &linearRec{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode graphql: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rec.queries = append(rec.queries, body.Query)
		w.Header().Set("Content-Type", "application/json")
		var vars struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body.Variables, &vars)
		if strings.Contains(body.Query, "query Issue(") {
			switch vars.ID {
			case "FIX-1":
				_, _ = w.Write([]byte(`{"data":{"issue":{"id":"iss-1","identifier":"FIX-1",` +
					`"assignee":{"id":"lin-u1","name":"dana","displayName":"Dana Okafor","email":"dana@example.com"}}}}`))
			default:
				_, _ = w.Write([]byte(`{"data":{"issue":{"id":"iss-2","identifier":"FIX-2","assignee":null}}}`))
			}
			return
		}
		t.Errorf("unexpected query: %s", truncate(body.Query, 60))
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	c := linear.New("linear-test-key-not-a-real-secret")
	c.Endpoint = srv.URL
	c.Retries, c.Backoff = 1, 0
	w := &linearWriter{c: c}
	ctx := context.Background()

	u, err := w.IssueAssignee(ctx, "FIX-1")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.ID() != "lin-u1" || u.DisplayName != "Dana Okafor" {
		t.Fatalf("assignee = %+v; want the fixture's Dana Okafor (lin-u1)", u)
	}
	u, err = w.IssueAssignee(ctx, "FIX-2")
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("unassigned issue answered %+v; want nil", u)
	}
	if len(rec.queries) != 2 {
		t.Fatalf("IssueAssignee made %d queries (%v); want exactly one read per call", len(rec.queries), rec.queries)
	}
}

// recordingWriterWithAssignee adds the preflight face for the forwarding hit
// row; a bare recordingWriter is the miss row.
type recordingWriterWithAssignee struct {
	recordingWriter
	readKey string
}

func (w *recordingWriterWithAssignee) IssueAssignee(ctx context.Context, key string) (*User, error) {
	w.readKey = key
	u := User{AccountID: "acc-hc", DisplayName: "Dana Whitfield"}
	return &u, nil
}

func TestActorTrailerForwardsIssueAssignee(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")
	ctx := context.Background()

	// Hit: the wrapped writer still answers the read — without the
	// forwarding method, interface embedding drops the face and every
	// assign on an actor-resolved workspace would refuse as "this origin
	// cannot answer".
	inner := &recordingWriterWithAssignee{}
	hit := WithActorTrailer(inner, &config.Config{Site: "https://x.example.com"})
	r, err := AsIssueAssigneeReader(hit)
	if err != nil {
		t.Fatalf("IssueAssignee lost through the wrapper: %v", err)
	}
	u, err := r.IssueAssignee(ctx, "NMB-1")
	if err != nil || u == nil || u.ID() != "acc-hc" {
		t.Fatalf("forwarded read = %+v, %v", u, err)
	}
	if inner.readKey != "NMB-1" {
		t.Fatalf("the wrapper answered without reaching the wrapped writer")
	}

	// Miss: a writer without the face keeps the refusal sentence, from the
	// verb call the same way As*'s error would have said it.
	miss := WithActorTrailer(&recordingWriter{}, &config.Config{Site: "https://x.example.com"})
	rm, err := AsIssueAssigneeReader(miss)
	if err != nil {
		t.Fatalf("assertion itself now fails: %v", err)
	}
	if _, err := rm.IssueAssignee(ctx, "NMB-1"); err == nil || !strings.Contains(err.Error(), "cannot read one issue's current assignee") {
		t.Fatalf("miss sentence changed: %v", err)
	}
}
