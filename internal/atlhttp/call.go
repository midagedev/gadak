package atlhttp

import (
	"context"
	"encoding/json"
)

// Call is the JSON request envelope on top of Do: marshal body, send, hand
// a non-2xx status to mapErr, unmarshal a 2xx body into out. The
// Atlassian clients (internal/jira, internal/confluence) differ only in
// how an HTTP failure becomes an error — the mapping is the mapErr
// parameter, so the envelope has one owner here (GDK-1577). out may be nil
// to ignore a body; an empty body with a non-nil out is not an error.
func Call(ctx context.Context, cfg Config, method, path string, body, out any, mutating bool, mapErr func(status int, data []byte) error) error {
	var payload []byte
	hasBody := body != nil
	if hasBody {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	status, data, err := Do(ctx, cfg, method, path, payload, hasBody, mutating)
	if err != nil {
		return err
	}
	if status >= 300 {
		return mapErr(status, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
