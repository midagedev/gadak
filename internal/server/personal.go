package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/midagedev/gadak/internal/store"
)

// Personal state lives in the local database only. A loopback-bound server has
// nobody to authenticate, so none of these ever answer 401 or 403
// (contracts/api.md, "Personal state").

type savedView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	OwnerEmail *string         `json:"owner_email"`
	OwnerName  *string         `json:"owner_name"`
	Config     json.RawMessage `json:"config"`
	CreatedAt  *string         `json:"created_at"`
	UpdatedAt  *string         `json:"updated_at"`
}

// owner stamps the configured identity on every view. It is what makes the
// client show its delete affordance, which it gates on owner_email == me.email.
func (s *server) owner() (*string, *string) {
	cfg := s.config()
	if cfg.Email == "" {
		return nil, nil
	}
	name := cfg.Email
	for _, m := range cfg.Members {
		if m.Email == cfg.Email && m.DisplayName != "" {
			name = m.DisplayName
			break
		}
	}
	return &cfg.Email, &name
}

func (s *server) view(v store.SavedView) savedView {
	email, name := s.owner()
	return savedView{
		ID:         v.ID,
		Name:       v.Name,
		OwnerEmail: email,
		OwnerName:  name,
		Config:     v.Config,
		CreatedAt:  nilIfEmpty(v.CreatedAt),
		UpdatedAt:  nilIfEmpty(v.UpdatedAt),
	}
}

type sourceView struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config"`
	JQL         string          `json:"jql"`
	ExternalID  string          `json:"external_id,omitempty"`
	Favourite   bool            `json:"favourite"`
	Owner       string          `json:"owner,omitempty"`
	Applied     []string        `json:"applied"`
	Unsupported []string        `json:"unsupported"`
}

func (s *server) handleGetViews(w http.ResponseWriter, r *http.Request) {
	stored, err := s.db.SavedViews(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	views := make([]savedView, 0, len(stored))
	for _, v := range stored {
		views = append(views, s.view(v))
	}
	src, err := s.db.SourceQueries(r.Context(), "jira")
	if err != nil {
		serverError(w, r, err)
		return
	}
	source := make([]sourceView, 0, len(src))
	for _, q := range src {
		source = append(source, sourceView{
			ID: q.ID, Name: q.Name, Config: q.Config, JQL: q.QueryText,
			ExternalID: q.ExternalID,
			Favourite:  q.Favourite, Owner: q.Owner,
			Applied: q.Applied, Unsupported: q.Unsupported,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": views, "source": source})
}

func (s *server) handlePostView(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.Name == "" {
		fail(w, http.StatusBadRequest, "name_required")
		return
	}
	v := store.SavedView{ID: newID(), Name: body.Name, Config: body.Config}
	if err := s.db.PutSavedView(r.Context(), v); err != nil {
		serverError(w, r, err)
		return
	}
	stored, err := s.db.SavedViews(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	for _, sv := range stored {
		if sv.ID == v.ID {
			writeJSON(w, http.StatusCreated, s.view(sv))
			return
		}
	}
	writeJSON(w, http.StatusCreated, s.view(v))
}

func (s *server) handleDeleteView(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteSavedView(r.Context(), r.PathValue("id")); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleGetWatches(w http.ResponseWriter, r *http.Request) {
	keys, err := s.db.Watches(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *server) handleDeleteWatch(w http.ResponseWriter, r *http.Request) {
	s.setWatch(w, r, r.PathValue("key"), false)
}

// setWatch takes the key as an argument rather than reading it from the route:
// the PUT route it shares with the assignee endpoint names its wildcards
// differently (see New).
func (s *server) setWatch(w http.ResponseWriter, r *http.Request, key string, on bool) {
	if err := s.db.SetWatch(r.Context(), key, on); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleGetFavorites(w http.ResponseWriter, r *http.Request) {
	keys, err := s.db.Favorites(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *server) handleDeleteFavorite(w http.ResponseWriter, r *http.Request) {
	s.setFavorite(w, r, r.PathValue("key"), false)
}

// setFavorite mirrors setWatch: the PUT path shares wildcards with assignee
// (see New), so the issue key is passed in rather than read from the route.
func (s *server) setFavorite(w http.ResponseWriter, r *http.Request, key string, on bool) {
	if err := s.db.SetFavorite(r.Context(), key, on); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return store.Now() // unique enough for a local single-user store
	}
	return hex.EncodeToString(b)
}
