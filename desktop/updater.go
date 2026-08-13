// In-app self-update, on wails v3's updater package: check GitHub Releases,
// download the desktop zip, verify it against SHA256SUMS, swap the bundle and
// relaunch. The server-side update check (the sidebar banner) still runs and
// still only tells you a release exists; this is the part that installs it.
package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	ghprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	// updateRepo is the releases feed the updater reads.
	updateRepo = "midagedev/gadak"
	// updateChecksums is the sidecar the provider parses for the zip's sha256.
	// Written by desktop/build-app.sh --dmg; goreleaser's own checksums.txt
	// covers the CLI archives and is a different file.
	updateChecksums = "SHA256SUMS"
)

// releaseVersion matches the version string only when this binary *is* a
// published release: "0.11.0" or "v0.11.0". The link-time default ("dev") and
// `git describe` output for a commit past a tag ("v0.10.0-15-gabc1234") both
// fail it. That is the gate — a build nobody published has no business
// replacing itself with one that was.
var releaseVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// initUpdater configures app.Updater and returns it, or returns nil when this
// build does not self-update. Every failure path returns nil after logging:
// an updater that cannot be configured must never keep the app from starting.
func initUpdater(app *application.App, version string) *updater.Updater {
	if !releaseVersion.MatchString(version) {
		log.Printf("updater disabled: %q is not a release version", version)
		return nil
	}
	gh, err := ghprovider.New(ghprovider.Config{
		Repository:    updateRepo,
		ChecksumAsset: updateChecksums,
		AssetMatcher:  desktopAssetMatcher,
	})
	if err != nil {
		log.Printf("updater disabled: github provider: %v", err)
		return nil
	}
	if err := app.Updater.Init(updater.Config{
		// Releases are tagged v1.2.3; the updater compares bare semver.
		CurrentVersion: strings.TrimPrefix(version, "v"),
		Providers:      []updater.Provider{gh},
		// No PublicKey: v1 verification is digest-only against SHA256SUMS.
		// The .app inside the zip is signed and notarized either way, so
		// Gatekeeper checks it again on launch. Ed25519 release signing is a
		// separate track.
	}); err != nil {
		log.Printf("updater disabled: %v", err)
		return nil
	}
	log.Printf("updater ready (%s, %s)", updateRepo, version)
	return app.Updater
}

// desktopAssetMatcher picks the desktop bundle zip by exact name. The
// package's DefaultAssetMatcher takes the first asset whose filename contains
// both GOOS and GOARCH, and on a gadak release that is goreleaser's CLI archive
// (gadak_0.11.0_darwin_arm64.tar.gz), which unpacks to a bare CLI binary rather
// than an .app. Matching one name is also fail-closed: a release with no
// desktop zip yields -1, which reads as "nothing to install here" instead of
// installing the wrong artifact.
func desktopAssetMatcher(req updater.CheckRequest, assets []ghprovider.ReleaseAsset) int {
	want := desktopAssetName(req.Platform, req.Arch)
	for i, a := range assets {
		if a.Name == want {
			return i
		}
	}
	return -1
}

// desktopAssetName is the release asset build-app.sh --dmg produces. Keep the
// two in step: this string is the whole handshake between them.
func desktopAssetName(platform, arch string) string {
	return fmt.Sprintf("gadak-desktop-%s-%s.zip", platform, arch)
}

// appendCheckForUpdatesMenu puts "Check for Updates…" at the top of Tools,
// above Install Command Line Tool….
func appendCheckForUpdatesMenu(appMenu *application.Menu, up *updater.Updater) {
	item := application.NewMenuItem("Check for Updates…").OnClick(func(*application.Context) {
		// CheckAndInstall does a network round trip and then drives the
		// framework's update window through download, verify and install.
		// The menu callback cannot wait for that.
		go func() {
			if err := up.CheckAndInstall(context.Background()); err != nil {
				log.Printf("update check: %v", err)
			}
		}()
	})
	toolsSubmenu(appMenu).Prepend(application.NewMenuFromItems(item))
}

// toolsSubmenu returns the Tools submenu, creating it if the platform-specific
// menu builders have not (appendInstallCLIMenu is macOS-only).
func toolsSubmenu(appMenu *application.Menu) *application.Menu {
	if item := appMenu.FindByLabel("Tools"); item != nil && item.IsSubmenu() {
		return item.GetSubmenu()
	}
	return appMenu.AddSubmenu("Tools")
}

// checkForUpdatesQuietly is the startup check. Check() does the whole round
// trip with no UI at all, so a machine already on the latest release sees
// nothing — no window, no flicker. Only when there is something to install do
// we hand over to CheckAndInstall, which opens the framework's window and
// re-runs the check inside it (one extra request, on the rare path).
func checkForUpdatesQuietly(ctx context.Context, up *updater.Updater) {
	rel, err := up.Check(ctx)
	if err != nil {
		log.Printf("update check: %v", err)
		return
	}
	if rel == nil {
		return
	}
	log.Printf("update available: %s", rel.Version)
	if err := up.CheckAndInstall(ctx); err != nil {
		log.Printf("update install: %v", err)
	}
}
