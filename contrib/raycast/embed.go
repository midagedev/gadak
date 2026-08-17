// Package raycastext embeds the Raycast extension source so the gadak
// binary can install it without a checkout (gadak raycast install).
package raycastext

import "embed"

//go:embed src assets package.json package-lock.json tsconfig.json eslint.config.mjs README.md CHANGELOG.md
var FS embed.FS
