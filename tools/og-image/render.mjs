/**
 * Renders the GitHub social-preview image (1280x640) with Playwright.
 * Delegates to the brand renderer so the card and the app icon stay one mark.
 *
 * Usage: node tools/og-image/render.mjs [outPath]
 */
import { spawnSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const result = spawnSync(process.execPath, [join(here, '../brand/render.mjs')], {
  stdio: 'inherit',
})
if (result.status) process.exit(result.status)
if (process.argv[2]) {
  const { copyFileSync, mkdirSync } = await import('node:fs')
  mkdirSync(dirname(process.argv[2]), { recursive: true })
  copyFileSync(join(here, '../../docs/media/og.png'), process.argv[2])
  console.log(process.argv[2])
}
