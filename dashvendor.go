// Vendor assets for agent-authored dashboards (GDK-792): the pinned chart
// library a dashboard may load from /api/v1/dashboards/vendor/ instead of a
// CDN. Embedding it keeps the dashboards CSP free of external hosts (no
// outbound rule change — the bytes ship inside the binary), and pinning the
// version once means every dashboard sees the same library instead of
// whatever a CDN serves that day.
//
// [GDK-808] three.js no longer ships embedded (−750 KB): anything bigger or
// less universal than uPlot belongs in the user-managed lib cache
// (`gadak dashboards lib add <url>` — sha384-pinned at download, re-hashed
// at serve, see internal/dashboards/libs.go), not in every binary gadak
// ships. uPlot stays embedded: one chart library is the norm the example
// dashboard sets.
//
// The HTTP whitelist is DashVendorFile's table, not the directory: the
// license is embedded for NOTICE/NPB purposes but is not served.
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
	"uPlot.iife.min.js": "text/javascript; charset=utf-8",
	"uPlot.min.css":     "text/css; charset=utf-8",
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
