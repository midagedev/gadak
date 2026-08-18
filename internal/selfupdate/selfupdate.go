// Package selfupdate answers one question — is a newer release published? —
// with one GitHub API call a day, cached on disk. It never fails loudly:
// update notice is a courtesy, not a feature the tool depends on.
package selfupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Info is the cached answer.
type Info struct {
	Latest    string `json:"latest"`     // tag without leading v, e.g. "0.3.1"
	URL       string `json:"url"`        // release html_url
	CheckedAt string `json:"checked_at"` // RFC3339 UTC
}

// APIBase is the GitHub API origin + repo path for release lookups.
// Tests replace it with an httptest server URL.
var APIBase = "https://api.github.com/repos/midagedev/gadak"

const (
	cacheName = "update-check.json"
	cacheTTL  = 24 * time.Hour
	httpTO    = 5 * time.Second
)

// Check returns the latest release info, from cache when fresh (<24h), else
// from GitHub. current=="0.0.0-dev" or cfg opt-out → ("", ok=false) without
// any network. Any error → ok=false, silently.
func Check(ctx context.Context, cacheDir, current string, enabled bool) (Info, bool) {
	if !enabled || current == "" || current == "0.0.0-dev" {
		return Info{}, false
	}
	if cacheDir == "" {
		return Info{}, false
	}

	if info, ok := readFreshCache(cacheDir); ok {
		return info, true
	}

	info, err := fetchLatest(ctx)
	if err != nil {
		return Info{}, false
	}
	info.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	_ = writeCache(cacheDir, info)
	return info, true
}

// Newer reports whether latest > current, comparing dotted numeric parts.
// Pre-release/dev suffixes are not newer than anything.
func Newer(current, latest string) bool {
	cp, ok := versionParts(current)
	if !ok {
		return false
	}
	lp, ok := versionParts(latest)
	if !ok {
		return false
	}
	n := len(cp)
	if len(lp) > n {
		n = len(lp)
	}
	for i := 0; i < n; i++ {
		var c, l int
		if i < len(cp) {
			c = cp[i]
		}
		if i < len(lp) {
			l = lp[i]
		}
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}

func versionParts(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, false
	}
	raw := strings.Split(v, ".")
	out := make([]int, len(raw))
	for i, p := range raw {
		if p == "" {
			return nil, false
		}
		// Non-numeric (including "0-dev", "3-rc1") → incomparable.
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		if n < 0 {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

func cachePath(dir string) string {
	return filepath.Join(dir, cacheName)
}

// DropCache removes the cached release so the next Check hits the network
// even inside the 24h TTL — what a person means by "check for updates now".
// A missing file is success. Callers must not rebuild the filename
// themselves: this package owns it, and a copy of the literal elsewhere
// would go on pointing at the old name after a rename here.
func DropCache(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.Remove(cachePath(dir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readFreshCache(dir string) (Info, bool) {
	data, err := os.ReadFile(cachePath(dir))
	if err != nil {
		return Info{}, false
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, false
	}
	if info.Latest == "" || info.CheckedAt == "" {
		return Info{}, false
	}
	checked, err := time.Parse(time.RFC3339, info.CheckedAt)
	if err != nil {
		return Info{}, false
	}
	if time.Since(checked) >= cacheTTL {
		return Info{}, false
	}
	return info, true
}

func writeCache(dir string, info Info) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(dir), data, 0o600)
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func fetchLatest(ctx context.Context) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTO)
	defer cancel()

	url := strings.TrimRight(APIBase, "/") + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Info{}, err
	}
	// No Authorization, no custom headers, no query string — identity-free GET.

	client := &http.Client{Timeout: httpTO}
	resp, err := client.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Info{}, errStatus(resp.StatusCode)
	}
	var body ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Info{}, err
	}
	tag := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if tag == "" || body.HTMLURL == "" {
		return Info{}, errEmpty
	}
	return Info{Latest: tag, URL: body.HTMLURL}, nil
}

type silentError string

func (e silentError) Error() string { return string(e) }

const errEmpty silentError = "selfupdate: empty release"

func errStatus(code int) error {
	return silentError("selfupdate: status " + strconv.Itoa(code))
}
