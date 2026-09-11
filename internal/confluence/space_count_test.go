package confluence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
)

/*
 * GDK-965: SpacePageCount is one request per space and never reports a
 * number it cannot verify.
 *
 *   TestSpacePageCount — one request, self-verifying totalSize
 *        ① totalSize above the limit → the real total
 *        ② totalSize 1 with no next link → exactly one page
 *        ③ totalSize 0 → an empty space is an answer, not a failure
 *        ④ totalSize 1 with a next link behind it → the echo shape, refused
 *        ⑤ totalSize absent → refused (not in the documented schema
 *           everywhere)
 *        ⑥ a 5xx → refused
 *        ⑦ every case issues exactly one search request, with the sync
 *           pass's CQL (space=<key> AND type=page) and limit=1
 */

func TestSpacePageCount(t *testing.T) {
	cases := []struct {
		name   string
		body   map[string]any
		status int
		wantN  int
		wantOK bool
	}{
		{
			name: "real total above the limit",
			body: map[string]any{
				"results":   []map[string]any{{"id": "1"}},
				"_links":    map[string]any{"next": "/rest/api/content/search?cql=x&limit=1&cursor=more"},
				"totalSize": 27,
			},
			wantN: 27, wantOK: true,
		},
		{
			name: "exactly one page",
			body: map[string]any{
				"results":   []map[string]any{{"id": "1"}},
				"_links":    map[string]any{},
				"totalSize": 1,
			},
			wantN: 1, wantOK: true,
		},
		{
			name: "empty space",
			body: map[string]any{
				"results":   []map[string]any{},
				"_links":    map[string]any{},
				"totalSize": 0,
			},
			wantN: 0, wantOK: true,
		},
		{
			name: "echoed size with more behind it",
			body: map[string]any{
				"results":   []map[string]any{{"id": "1"}},
				"_links":    map[string]any{"next": "/rest/api/content/search?cql=x&limit=1&cursor=more"},
				"totalSize": 1,
			},
			wantN: 0, wantOK: false,
		},
		{
			name: "totalSize absent",
			body: map[string]any{
				"results": []map[string]any{{"id": "1"}},
				"_links":  map[string]any{},
			},
			wantN: 0, wantOK: false,
		},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
			wantN:  0, wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits int32
			var gotQuery string
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				gotQuery = r.URL.RawQuery
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			// One attempt for the error case too: the count's own contract
			// is one search call, and the transport's retry ladder (4 in
			// testClient) is not this test's subject.
			if tc.status != 0 {
				c.Retries = 1
			}
			n, ok := c.SpacePageCount(context.Background(), "DEV")
			if ok != tc.wantOK || n != tc.wantN {
				t.Fatalf("SpacePageCount = (%d, %v), want (%d, %v)", n, ok, tc.wantN, tc.wantOK)
			}
			if h := atomic.LoadInt32(&hits); h != 1 {
				t.Fatalf("%d search requests for one space, want exactly 1", h)
			}
			q, err := url.ParseQuery(gotQuery)
			if err != nil {
				t.Fatalf("query %q did not parse: %v", gotQuery, err)
			}
			if got := q.Get("cql"); got != `space="DEV" AND type=page` {
				t.Fatalf("cql = %q, want the sync pass's space+type CQL", got)
			}
			if got := q.Get("limit"); got != "1" {
				t.Fatalf("limit = %q, want 1 (the self-verifying single-row ask)", got)
			}
		})
	}
}
