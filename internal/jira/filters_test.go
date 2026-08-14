package jira

import (
	"context"
	"net/http"
	"testing"
)

func TestMyFiltersDecodesOwnedAndStarred(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/filter/my" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("includeFavourites") != "true" {
			t.Errorf("includeFavourites %q", r.URL.Query().Get("includeFavourites"))
		}
		w.Write([]byte(`[
			{"id":"10000","name":"All Open Bugs","jql":"type = Bug AND resolution is EMPTY","favourite":true,
			 "owner":{"displayName":"Mia Krystof"}},
			{"id":"10010","name":"My issues","jql":"assignee = currentUser()","favourite":false,
			 "owner":{"displayName":"Mia Krystof"}}
		]`))
	}))
	got, err := c.MyFilters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].ID != "10000" || got[0].JQL == "" || !got[0].Favourite || got[0].Owner != "Mia Krystof" {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].ID != "10010" || got[1].Favourite {
		t.Fatalf("second %+v", got[1])
	}
}
