package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestCreateFieldsReturnsRequiredSet(t *testing.T) {
	f, h, _ := writable(t)
	f.createFieldsJSON = `{"maxResults":50,"startAt":0,"total":6,"fields":[
		{"fieldId":"issuetype","name":"Issue Type","required":true,"hasDefaultValue":false,"schema":{"type":"issuetype"}},
		{"fieldId":"project","name":"Project","required":true,"hasDefaultValue":false,"schema":{"type":"project"}},
		{"fieldId":"reporter","name":"Reporter","required":true,"hasDefaultValue":true,"schema":{"type":"user"}},
		{"fieldId":"summary","name":"요약","required":true,"hasDefaultValue":false,"schema":{"type":"string"}},
		{"fieldId":"customfield_10050","name":"Sprint","required":true,"hasDefaultValue":false,"schema":{"type":"array"}},
		{"fieldId":"duedate","name":"Due date","required":false,"hasDefaultValue":false,"schema":{"type":"date"}}
	]}`

	rec := get(t, h, apiBase+"create-meta/fields/?project=NMB&issue_type=10004", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Fields []struct {
			FieldID    string `json:"field_id"`
			Name       string `json:"name"`
			Required   bool   `json:"required"`
			HasDefault bool   `json:"has_default"`
			Type       string `json:"type"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Fields) != 6 {
		t.Fatalf("fields %+v, want all 6 (optional duedate included — not required-only)", body.Fields)
	}
	byID := map[string]struct {
		Name       string
		Required   bool
		HasDefault bool
		Type       string
	}{}
	for _, f := range body.Fields {
		byID[f.FieldID] = struct {
			Name       string
			Required   bool
			HasDefault bool
			Type       string
		}{f.Name, f.Required, f.HasDefault, f.Type}
	}
	sum, ok := byID["summary"]
	if !ok || !sum.Required || sum.HasDefault || sum.Type != "string" || sum.Name != "요약" {
		t.Errorf("summary %+v", sum)
	}
	rep, ok := byID["reporter"]
	if !ok || !rep.Required || !rep.HasDefault {
		t.Errorf("reporter %+v", byID["reporter"])
	}
	if _, ok := byID["customfield_10050"]; !ok {
		t.Errorf("missing customfield_10050: %+v", body.Fields)
	}
	due, ok := byID["duedate"]
	if !ok || due.Required || due.Type != "date" {
		t.Errorf("optional duedate %+v — server must not drop non-required fields", due)
	}
	called := false
	for _, c := range f.calls {
		if strings.Contains(c, "/issue/createmeta/") && strings.Contains(c, "/issuetypes/") {
			called = true
			break
		}
	}
	if !called {
		t.Errorf("origin was not asked for createmeta fields: %v", f.calls)
	}
}

func TestCreateFieldsMissingQuery(t *testing.T) {
	_, h, _ := writable(t)
	for _, path := range []string{
		apiBase + "create-meta/fields/",
		apiBase + "create-meta/fields/?project=NMB",
		apiBase + "create-meta/fields/?issue_type=10004",
	} {
		rec := get(t, h, path, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s → %d %s, want 400", path, rec.Code, rec.Body.String())
			continue
		}
		if got := decode[map[string]string](t, rec)["error"]; got != "project_and_issue_type_required" {
			t.Errorf("%s error %q", path, got)
		}
	}
}

func TestCreateFieldsNoCredential(t *testing.T) {
	db, _ := fixture(t)
	h := New(db, &config.Config{Projects: []string{"NMB"}})
	rec := get(t, h, apiBase+"create-meta/fields/?project=NMB&issue_type=10004", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d %s, want 409", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "credential_required" {
		t.Fatalf("error %q", got)
	}
}

func TestCreateFieldsOriginErrorDoesNot500(t *testing.T) {
	f, h, _ := writable(t)
	f.createFieldsStatus = http.StatusInternalServerError
	f.errBody = `{"errorMessages":["createmeta exploded"]}`

	rec := get(t, h, apiBase+"create-meta/fields/?project=NMB&issue_type=10004", nil)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("origin 500 became HTTP 500 %s — client cannot degrade", rec.Body.String())
	}
	if rec.Code < 400 {
		t.Fatalf("status %d %s, want a degradable error", rec.Code, rec.Body.String())
	}
	asked := false
	for _, c := range f.calls {
		if strings.Contains(c, "/issue/createmeta/") && strings.Contains(c, "/issuetypes/") {
			asked = true
			break
		}
	}
	if !asked {
		t.Fatalf("origin was not asked for createmeta fields: %v", f.calls)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] == nil || body["error"] == "" {
		t.Fatalf("degradable error body missing error: %s", rec.Body.String())
	}
}
