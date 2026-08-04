package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is a minimal Jira Cloud REST helper for seeding. It is intentionally
// separate from internal/jira so this tool can write without touching the
// read-only connector package.
type Client struct {
	base  string
	auth  string
	http  *http.Client
	tries int
	// backoff is the first wait; doubles each attempt, capped at 30s.
	// Zero means no sleep (tests).
	backoff time.Duration
}

func newClient(site, email, token string) *Client {
	return &Client{
		base:    strings.TrimRight(site, "/"),
		auth:    "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		http:    &http.Client{Timeout: 60 * time.Second},
		tries:   5,
		backoff: time.Second,
	}
}

// call performs one Jira REST request with 429/5xx retry. On success it
// unmarshals into out when non-nil and returns true. On failure it prints a
// short error (never including credentials) and returns false.
func (c *Client) call(method, path string, body any, out any) bool {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR marshal %s %s: %v\n", method, path, err)
			return false
		}
	}
	for attempt := 0; attempt < c.tries; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, c.base+path, rdr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR %s %s: %v\n", method, path, err)
			return false
		}
		req.Header.Set("Authorization", c.auth)
		req.Header.Set("Accept", "application/json")
		// English field/status names regardless of the account's display language.
		// (createmeta still localizes type names; see issueTypeIDs.)
		req.Header.Set("Accept-Language", "en")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.http.Do(req)
		if err != nil {
			if attempt < c.tries-1 {
				c.sleep(attempt)
				continue
			}
			fmt.Fprintf(os.Stderr, "  ERROR %s %s: %v\n", method, path, err)
			return false
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "  ERROR %s %s: %v\n", method, path, readErr)
			return false
		}
		if res.StatusCode == 429 || res.StatusCode == 500 || res.StatusCode == 502 ||
			res.StatusCode == 503 || res.StatusCode == 504 {
			if attempt < c.tries-1 {
				wait := c.waitFor(attempt)
				fmt.Fprintf(os.Stderr, "  retry %d in %s: %s %s\n", res.StatusCode, wait, method, path)
				if wait > 0 {
					time.Sleep(wait)
				}
				continue
			}
		}
		if res.StatusCode >= 300 {
			detail := strings.TrimSpace(string(data))
			if len(detail) > 400 {
				detail = detail[:400]
			}
			fmt.Fprintf(os.Stderr, "  ERROR %d %s %s: %s\n", res.StatusCode, method, path, detail)
			return false
		}
		if out == nil || len(data) == 0 {
			return true
		}
		if err := json.Unmarshal(data, out); err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR decode %s %s: %v\n", method, path, err)
			return false
		}
		return true
	}
	return false
}

func (c *Client) waitFor(attempt int) time.Duration {
	if c.backoff <= 0 {
		return 0
	}
	d := c.backoff << attempt
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func (c *Client) sleep(attempt int) {
	if d := c.waitFor(attempt); d > 0 {
		time.Sleep(d)
	}
}
