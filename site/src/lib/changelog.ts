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
  ja: '../../../CHANGELOG.ja.md',
}

/**
 * True when this locale's page renders the English file rather than its own.
 *
 * No locale does today: ja got its own `CHANGELOG.ja.md` on 2026-09-09. The
 * check stays because a fourth locale would arrive without one, and the page
 * has to say so rather than pass English off as a translation.
 */
export function changelogIsFallback(lang: Locale): boolean {
  return lang !== 'en' && FILES[lang] === FILES.en
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
 *
 * A release is a heading whose label is a version. That test lives here and
 * nowhere else (GDK-2028): each of the three locale pages used to narrow this
 * list itself with `releases.filter((r) => r.date)`, reading "has a date" as
 * a proxy for "is a release" — and on 2026-09-21 the proxy failed. The three
 * newest releases had shipped with no date in their heading, so v0.24.0,
 * v0.23.1 and v0.23.0 had no row in the jump list on any of the three pages,
 * and the JSON-LD told search engines the current version was 0.22.1. The
 * date is what a row *shows*; it was never what makes a heading a release.
 *
 * `Unreleased` is excluded, which is what the old filter got right by
 * accident — it is the one H2 the page carries that has not shipped.
 */
function readReleases(html: string): Release[] {
  const out: Release[] = []
  const heading = /<h2\b[^>]*\bid="([^"]+)"[^>]*>([\s\S]*?)<\/h2>/g
  for (const [, id, inner] of html.matchAll(heading)) {
    const text = inner.replace(/<[^>]+>/g, '').trim()
    if (!text) continue
    const [label, date] = text.split(/\s+[—–-]\s+/, 2)
    const version = label.trim()
    if (!/^v\d/.test(version)) continue
    out.push({ id, label: version, date: date?.trim() })
  }
  return out
}

/**
 * Render markdown with the site's one processor, so heading ids,
 * smartypants, and link syntax behave identically everywhere this site
 * serves prose. A handful of calls per build; the processor is created
 * per call because that is the supported shape for ad-hoc rendering.
 */
export async function renderMarkdown(md: string): Promise<string> {
  const processor = await createMarkdownProcessor({})
  const { code } = await processor.render(md)
  return code
}

/**
 * Japanese sets solid, and a soft line break does not.
 *
 * CommonMark keeps a wrapped line's newline, HTML collapses it to a space,
 * and the changelog files are wrapped at 80 columns — so every wrap point
 * inside a Japanese paragraph grew a space that is invisible in the file and
 * visible on the page. Measured 2026-09-13: 16 of them on /ja/changelog/,
 * which is what a reader saw as the page being spaced like Korean.
 *
 * The join is per line pair and only where both sides are Japanese, so a
 * wrap between Latin words still renders as the space it means. Fenced code
 * is left alone: a break inside a fence is a line of the program.
 *
 * This is the rendering half of the rule. The other half — that the file's
 * own prose puts no ASCII space beside a Japanese character — is
 * tools/ja-spacing.py, which CHANGELOG.ja.md is under. The Japanese prose
 * that is not a page of this site, README.ja.md first among it, is not
 * (GDK-1855).
 */
const JA_TAIL = /[\u3000-\u303f\u3040-\u30ff\u3400-\u9fff\uff01-\uff60]$/
const JA_HEAD = /^[\u3000-\u303f\u3040-\u30ff\u3400-\u9fff\uff01-\uff60]/
/** Emphasis markers render as nothing, so they do not decide adjacency:
 *  a paragraph opening `**…できます。**` still ends in Japanese. */
function rendered(line: string, end: 'tail' | 'head' = 'tail'): string {
  return end === 'tail' ? line.trimEnd().replace(/[*_]+$/, '') : line.trimStart().replace(/^[*_]+/, '')
}

/** A line that opens a block of its own is never a continuation of the one above. */
const BLOCK = /^\s*(#{1,6}\s|[-*+]\s|\d+\.\s|>|\||\[[^\]]*\]:|<|=|```|~~~)/

export function joinJapaneseWraps(md: string): string {
  const out: string[] = []
  let fenced = false
  for (const line of md.split('\n')) {
    if (/^\s*(```|~~~)/.test(line)) fenced = !fenced
    const prev = out[out.length - 1]
    const joinable =
      !fenced &&
      prev !== undefined &&
      prev.trim() !== '' &&
      line.trim() !== '' &&
      !BLOCK.test(line) &&
      !/^\s*#{1,6}\s/.test(prev) &&
      // Either side being Japanese is enough. The wrap points are where the
      // 80-column wrapper found a space, and after tools/ja-spacing.py the
      // only spaces left in Japanese prose are the ones inside Latin runs —
      // so a break with Japanese on one side is a break the file put in the
      // middle of solid text.
      (JA_TAIL.test(rendered(prev)) || JA_HEAD.test(rendered(line, 'head')))
    if (joinable) {
      out[out.length - 1] = prev.trimEnd() + line.trimStart()
      continue
    }
    out.push(line)
  }
  return out.join('\n')
}

export async function renderChangelog(
  lang: Locale,
): Promise<{ html: string; releases: Release[] }> {
  const raw = readFileSync(new URL(FILES[lang], import.meta.url), 'utf8')
  const prepared = lang === 'ja' ? joinJapaneseWraps(stripTitleBlock(raw)) : stripTitleBlock(raw)
  const html = await renderMarkdown(prepared)
  return { html, releases: readReleases(html) }
}
