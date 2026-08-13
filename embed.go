// Package gadak embeds the built web UI so a release is one self-contained
// binary. `npm run build` writes web assets to dist/app before `go build`;
// without that step the embed carries only the committed placeholder and
// WebUI reports ok=false, which `gadak serve` turns into a helpful error.
//
// It also embeds the Claude Code skill (skills/gadak/SKILL.md) so
// `gadak skill install` works for brew installs without a source checkout.
package gadak

import (
	"embed"
	"io/fs"
)

//go:embed all:dist/app
var distFS embed.FS

//go:embed skills/gadak/SKILL.md
var skillMarkdown []byte

// WebUI returns the embedded web assets rooted at the app directory. ok is
// false when the binary was built without a web build (placeholder only).
func WebUI() (fs.FS, bool) {
	sub, err := fs.Sub(distFS, "dist/app")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return sub, false
	}
	return sub, true
}

// SkillMarkdown returns the embedded Claude Code skill body (skills/gadak/SKILL.md).
// Callers must not modify the returned slice.
func SkillMarkdown() []byte {
	return skillMarkdown
}
