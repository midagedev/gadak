import { readFileSync } from 'node:fs'

import { createMarkdownProcessor } from '@astrojs/markdown-remark'

import type { Locale } from '../i18n'

/**
 * The site's changelog page is the repository's CHANGELOG, rendered — not a
 * second copy of it.
 *
 * CHANGELOG.md is history and is never edited in place (see CLAUDE.md), so a
 * hand-maintained web version would drift the moment anyone shipped. Reading
 * the file at build time means the page is wrong only if the file is, and the
 * release commit that touches the changelog is also what redeploys the page.
 *
 * The file lives at the repo root, one level above the Astro project. Vite
 * would need `fs.allow` widened to import it as a module, so it is read
 * directly instead and run through the same processor Astro uses for pages in
 * src/ — same heading ids, same smartypants, no new dependency.
 */
const FILES: Record<Locale, string> = {
  en: '../../../CHANGELOG.md',
  ko: '../../../CHANGELOG.ko.md',
}

/**
 * Drop the file's own title block: an `# Changelog` H1 (the page supplies its
 * own, with a lede) and the `<sub>` line under it, whose language links are
 * relative paths to the *other markdown file* and would 404 on the site. The
 * header's language switcher already does that job.
 */
function stripTitleBlock(md: string): string {
  return md.replace(/^#\s+Changelog\s*\n+(<sub>[\s\S]*?<\/sub>\s*\n+)?/, '')
}

/** One released version, as the page's jump list and its JSON-LD need it. */
export interface Release {
  /** Heading id, for the in-page anchor. */
  id: string
  /** "v0.17.3" — or the locale's word for the unreleased section. */
  label: string
  /** ISO date, when the heading carries one. */
  date?: string
}

/**
 * Pull the version index out of the rendered HTML rather than out of the
 * markdown: the ids are what the anchors have to match, and the processor —
 * not this file — is what assigns them.
 */
function readReleases(html: string): Release[] {
  const out: Release[] = []
  const heading = /<h2\b[^>]*\bid="([^"]+)"[^>]*>([\s\S]*?)<\/h2>/g
  for (const [, id, inner] of html.matchAll(heading)) {
    const text = inner.replace(/<[^>]+>/g, '').trim()
    if (!text) continue
    const [label, date] = text.split(/\s+[—–-]\s+/, 2)
    out.push({ id, label: label.trim(), date: date?.trim() })
  }
  return out
}

export async function renderChangelog(
  lang: Locale,
): Promise<{ html: string; releases: Release[] }> {
  const raw = readFileSync(new URL(FILES[lang], import.meta.url), 'utf8')
  const processor = await createMarkdownProcessor({})
  const { code } = await processor.render(stripTitleBlock(raw))
  return { html: code, releases: readReleases(code) }
}
