package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Live-shaped createmeta field (GDK-254, measured 2026-08-21): a list
// element, not an editmeta map value. fieldId is on the object.
func createFieldJSON(fieldID, name string, required, hasDefault bool, schemaType string) string {
	return fmt.Sprintf(
		`{"fieldId":%s,"name":%s,"required":%t,"hasDefaultValue":%t,"schema":{"type":%s},"operations":["set"]}`,
		jsonString(fieldID), jsonString(name), required, hasDefault, jsonString(schemaType),
	)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestCreateFieldsParsesFieldIdRequiredAndDefault(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" && strings.Contains(r.URL.RawQuery, "expand=") {
			t.Errorf("deprecated expand query: %s", r.URL.RawQuery)
		}
		wantPath := "/rest/api/3/issue/createmeta/GDK/issuetypes/10003"
		if r.URL.Path != wantPath {
			t.Errorf("path %s, want %s", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"total":4,"fields":[` +
			createFieldJSON("issuetype", "Issue Type", true, false, "issuetype") + `,` +
			createFieldJSON("project", "Project", true, false, "project") + `,` +
			createFieldJSON("reporter", "Reporter", true, true, "user") + `,` +
			createFieldJSON("summary", "요약", true, false, "string") +
			`]}`))
	}))
	got, err := c.CreateFields(context.Background(), "GDK", "10003")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len=%d, want 4: %+v", len(got), got)
	}
	byID := map[string]CreateFieldMeta{}
	for _, f := range got {
		byID[f.FieldID] = f
	}
	rep, ok := byID["reporter"]
	if !ok {
		t.Fatalf("missing reporter: %+v", got)
	}
	if !rep.Required || !rep.HasDefaultValue || rep.Schema.Type != "user" {
		t.Errorf("reporter %+v, want required+hasDefault+user", rep)
	}
	sum, ok := byID["summary"]
	if !ok {
		t.Fatalf("missing summary: %+v", got)
	}
	if !sum.Required || sum.HasDefaultValue || sum.Name != "요약" || sum.Schema.Type != "string" {
		t.Errorf("summary %+v", sum)
	}
}

func TestCreateFieldsPagesUntilTotal(t *testing.T) {
	var starts []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/createmeta/") {
			t.Errorf("path %s", r.URL.Path)
		}
		if strings.Contains(r.URL.RawQuery, "expand=") {
			t.Errorf("deprecated expand: %s", r.URL.RawQuery)
		}
		q := r.URL.Query()
		starts = append(starts, q.Get("startAt"))
		w.Header().Set("Content-Type", "application/json")
		switch q.Get("startAt") {
		case "", "0":
			// Page 1: always-sent + reporter. The page-2 required field is
			// absent on purpose — a one-page client fails the assertion below.
			_, _ = w.Write([]byte(`{"maxResults":2,"startAt":0,"total":3,"fields":[` +
				createFieldJSON("summary", "Summary", true, false, "string") + `,` +
				createFieldJSON("reporter", "Reporter", true, true, "user") +
				`]}`))
		case "2":
			_, _ = w.Write([]byte(`{"maxResults":2,"startAt":2,"total":3,"fields":[` +
				createFieldJSON("customfield_10050", "Sprint", true, false, "array") +
				`]}`))
		default:
			t.Errorf("unexpected startAt %q", q.Get("startAt"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	got, err := c.CreateFields(context.Background(), "GDK", "10003")
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) < 2 {
		t.Fatalf("pages fetched=%d starts=%v — one-page clients miss page 2", len(starts), starts)
	}
	var found bool
	for _, f := range got {
		if f.FieldID == "customfield_10050" && f.Required && !f.HasDefaultValue && f.Name == "Sprint" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("page-2 required field customfield_10050 missing: %+v", got)
	}
}

func TestCreateFieldsPathEscapesProjectAndType(t *testing.T) {
	var sawURI string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawURI = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"total":0,"fields":[]}`))
	}))
	_, err := c.CreateFields(context.Background(), "NMB/X", "10 03")
	if err != nil {
		t.Fatal(err)
	}
	// r.URL.Path is decoded; the wire form is RequestURI (confluence client_test.go).
	want := "/rest/api/3/issue/createmeta/" + url.PathEscape("NMB/X") + "/issuetypes/" + url.PathEscape("10 03")
	if !strings.Contains(sawURI, want) {
		t.Errorf("RequestURI %s, want it to contain %s", sawURI, want)
	}
}
