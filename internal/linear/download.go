package linear

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// UploadsHost is the only Linear storage hostname allowed to receive the API
// key on a download GET. Subdomains and port variants are not this host;
// http is not this scheme.
const UploadsHost = "uploads.linear.app"

// IsUploadsURL reports whether target is an https URL whose host is exactly
// uploads.linear.app. Linear documents that downloads from this host accept
// the API key in Authorization. That is a different path from the upload PUT
// to a signed URL, which must not carry the key (write.go UploadFile).
//
// A stored URL is data the mirror copied from the origin, so it is never
// fetched unless it passes this (GDK-560: SSRF).
func IsUploadsURL(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Host == UploadsHost
}

// downloadClient refuses a hop off uploads.linear.app so Authorization
// cannot follow a subdomain redirect (GDK-558).
var downloadClient = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("linear: stopped after 10 redirects")
		}
		if !IsUploadsURL(req.URL.String()) {
			return fmt.Errorf("linear: refusing a redirect off %s", UploadsHost)
		}
		return nil
	},
}

// Download opens an attachment's bytes from Linear's storage host. The
// caller closes the body. A target that is not an uploads.linear.app https
// URL is refused before any request leaves the process.
func Download(ctx context.Context, target, apiKey string) (io.ReadCloser, string, error) {
	if !IsUploadsURL(target) {
		return nil, "", fmt.Errorf("linear: attachment URL is not on %s", UploadsHost)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}
	res, err := downloadClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, "", fmt.Errorf("linear: attachment download: HTTP %d", res.StatusCode)
	}
	return res.Body, res.Header.Get("Content-Type"), nil
}
