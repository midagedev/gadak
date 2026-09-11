// Package raycastext embeds the Raycast extension source so the gadak
// binary can install it without a checkout (gadak raycast install).
package raycastext

import "embed"

// .prettierrc rides along because `ray lint` runs Prettier over the
// installed tree: the sources are formatted at printWidth 120, so an
// install without it reports every file as unformatted.
//
//go:embed src assets package.json package-lock.json tsconfig.json eslint.config.mjs README.md CHANGELOG.md .prettierrc
var FS embed.FS
