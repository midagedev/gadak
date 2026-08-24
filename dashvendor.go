// Vendor assets for agent-authored dashboards (GDK-792): the pinned chart
// and 3D libraries a dashboard may load from /api/v1/dashboards/vendor/
// instead of a CDN. Embedding them keeps the dashboards CSP free of external
// hosts (no outbound rule change — the bytes ship inside the binary), and
// pinning the versions once means every dashboard sees the same library
// instead of whatever a CDN serves that day.
//
// The HTTP whitelist is DashVendorFile's table, not the directory: the
// licenses are embedded for NOTICE/NPB purposes but are not served.
package gadak

import (
	"embed"
)

//go:embed dashvendor
var dashVendorFS embed.FS

// dashVendorDir is the prefix to strip from embed paths.
const dashVendorDir = "dashvendor/"

// dashVendorFiles is the serving whitelist: filename → content type. It is
// deliberately exhaustive — adding a file to dashvendor/ without adding it
// here serves nothing (the route 404s), which is the fail-closed direction.
var dashVendorFiles = map[string]string{
	"uPlot.iife.min.js":   "text/javascript; charset=utf-8",
	"three.module.min.js": "text/javascript; charset=utf-8",
	"three.core.min.js":   "text/javascript; charset=utf-8",
	"uPlot.min.css":       "text/css; charset=utf-8",
}

// DashVendorFile returns the embedded bytes and content type for a whitelisted
// vendor filename. ok is false for anything not in the table — unknown names,
// subdirectories, license files — and the route answers 404 in that case.
// Callers must not modify the returned slice.
func DashVendorFile(name string) ([]byte, string, bool) {
	contentType, served := dashVendorFiles[name]
	if !served {
		return nil, "", false
	}
	body, err := dashVendorFS.ReadFile(dashVendorDir + name)
	if err != nil {
		return nil, "", false
	}
	return body, contentType, true
}
