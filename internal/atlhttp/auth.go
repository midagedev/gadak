package atlhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrAuth is the shared identity for a rejected Atlassian credential
// (HTTP 401 or 403). Client packages keep named wrappers (jira.ErrAuth,
// confluence.ErrAuth) so existing errors.Is call sites keep compiling;
// those wrappers unwrap to this sentinel.
//
// DoRaw never returns this: a completed HTTP response is status+body.
// JSON call helpers use Do, which classifies 401/403 here so a new
// client inherits the identity by using the transport.
var ErrAuth = errors.New("credential rejected")

// RejectedCredential is the marker Watch keys on. A source that is not
// built on this package can still implement the method; Do-produced
// errors implement it too.
type RejectedCredential interface {
	RejectedCredential()
}

// AuthError names which connector's credential died. Error() is
// "<prefix>: credential rejected" so last_error distinguishes sources.
// Unwrap returns ErrAuth so errors.Is(err, ErrAuth) is true for every client.
type AuthError struct {
	Prefix string
}

func (e AuthError) Error() string {
	if e.Prefix == "" {
		return ErrAuth.Error()
	}
	return e.Prefix + ": " + ErrAuth.Error()
}

func (e AuthError) Unwrap() error { return ErrAuth }

func (e AuthError) RejectedCredential() {}

// Auth returns a rejected-credential error named by prefix
// (Config.ErrPrefix: "jira", "confluence", …).
func Auth(prefix string) error {
	return AuthError{Prefix: prefix}
}

// authFromStatus returns Auth(prefix) for 401/403, nil otherwise.
func authFromStatus(status int, prefix string) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Auth(prefix)
	}
	return nil
}

// Do is DoRaw plus 401/403 classification. JSON call helpers use this so a
// new client inherits rejected-credential identity. Raw keeps DoRaw: a
// completed response is never an error there.
func Do(ctx context.Context, cfg Config, method, path string, payload []byte, hasBody, mutating bool) (int, []byte, error) {
	status, data, err := DoRaw(ctx, cfg, method, path, payload, hasBody, mutating)
	if err != nil {
		return status, data, err
	}
	if authErr := authFromStatus(status, cfg.ErrPrefix); authErr != nil {
		statusLine := fmt.Sprintf("%d %s", status, http.StatusText(status))
		return status, data, fmt.Errorf("%s %s: %w (%s)", method, path, authErr, statusLine)
	}
	return status, data, nil
}
