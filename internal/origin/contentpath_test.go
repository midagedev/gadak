package origin

import "testing"

func TestSiteRelativeKeepsTheCredentialOnTheSite(t *testing.T) {
	const base = "http://jira.example:2990/jira"

	got, err := SiteRelative(base, base+"/secure/attachment/10001/shot%20one.png")
	if err != nil {
		t.Fatal(err)
	}
	// Relative to the base: atlhttp concatenates, so the context path must
	// appear once in the final URL, not twice.
	if got != "/secure/attachment/10001/shot%20one.png" {
		t.Fatalf("path = %q — want it relative to the base, escaping intact", got)
	}
	// A path outside the site's own prefix is refused, not trimmed into one.
	if _, err := SiteRelative(base, "http://jira.example:2990/other/secure/attachment/1/x.png"); err == nil {
		t.Fatal("a URL outside the site path was accepted")
	}
	// A site with no context path keeps the whole path.
	got, err = SiteRelative("https://site.atlassian.net", "https://site.atlassian.net/secure/attachment/2/y.png")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/secure/attachment/2/y.png" {
		t.Fatalf("path = %q on a context-path-less site", got)
	}

	// Another host is refused, not followed.
	if _, err := SiteRelative(base, "http://elsewhere.example/secure/attachment/1/x.png"); err == nil {
		t.Fatal("an off-site content URL was accepted")
	}
	// So is another scheme on the same host: an https→http downgrade would
	// put the Authorization header on the wire in clear.
	if _, err := SiteRelative("https://jira.example/jira", "http://jira.example/jira/secure/attachment/1/x.png"); err == nil {
		t.Fatal("a scheme downgrade was accepted")
	}
	// An empty URL names the missing sync rather than requesting "".
	if _, err := SiteRelative(base, ""); err == nil {
		t.Fatal("an empty content URL was accepted")
	}
}
