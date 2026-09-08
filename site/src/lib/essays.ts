import { renderMarkdown } from './changelog'
import { LOCALES, type Locale } from '../i18n'

/**
 * The site's essays: long-form writing that lives at durable URLs instead
 * of decaying in a release comment.
 *
 * Source convention — one file per essay at src/content/essays/<slug>.md,
 * where <slug> is the URL:
 *
 *   ---
 *   title: "gadak 0.18: the Jira mirror that no longer needs Jira"
 *   date: 2026-08-28
 *   description: One line — the index list and og:description both use it.
 *   lang: en
 *   ---
 *
 * Frontmatter is a flat `key: value` block parsed here, not through a YAML
 * dependency: the schema is exactly these four single-line strings (quotes
 * optional; the first `: ` ends the key, so titles may contain colons).
 * Anything after the closing fence is markdown rendered by the same
 * processor as the changelog — which is also why issue keys link into the
 * public backlog exactly the way CHANGELOG.md does it, reference style:
 *
 *   shipped as [GDK-NNN].
 *   [GDK-NNN]: https://gadak.dev/backlog/#/?ks=GDK-NNN
 *
 * (NNN is the issue number — a real key, not a placeholder, when you write
 * the line; keys cited here must already be on the public backlog, or
 * tools/doc-checks.sh fails the build on a dangling reference.)
 *
 * Translations sit beside the original as <slug>.<lang>.md (ko, ja). The
 * English file is canonical and owns the slug; a translation is the same
 * essay in another locale, served at /<lang>/essays/<slug>/. An essay with
 * no translation stays en-only: its page passes noAltLang to the layout and
 * the sitemap claims no alternates for it — an hreflang that 404s is worse
 * than none. Adding an essay is dropping a file in the directory — the
 * loader is the registration, and a malformed file fails the build loudly;
 * a translation whose slug has no English original fails the same way.
 */

/**
 * The directory is read with import.meta.glob rather than node:fs: Astro's
 * build bundles each page into site/dist/pages/, where a path relative to
 * import.meta.url no longer points at src/ (the changelog's
 * ../../../CHANGELOG.md only survives its own bundling because src/lib and
 * dist/pages sit at the same depth). The glob is resolved by Vite at bundle
 * time, so the loader works identically in dev, build, and from any cwd.
 */
const FILES = import.meta.glob('../content/essays/*.md', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

/** One essay, as the index list and the essay page need it. */
export interface Essay {
  /** URL segment — the file name without .md. */
  slug: string
  title: string
  /** ISO date, printed as-is (the changelog prints ISO dates too). */
  date: string
  description: string
  /** The locale this text is in — the file's suffix, 'en' for the original. */
  lang: Locale
  /** Body, already rendered. */
  html: string
  /** Locales this essay exists in, en first — the layout's alternates. */
  locales: Locale[]
}

function parseFrontmatter(
  raw: string,
  slug: string,
): { fields: Record<string, string>; body: string } {
  const lines = raw.split(/\r?\n/)
  if (lines[0]?.trim() !== '---') {
    throw new Error(`essay ${slug}: expected the file to open with a '---' frontmatter fence`)
  }
  const end = lines.indexOf('---', 1)
  if (end === -1) {
    throw new Error(`essay ${slug}: frontmatter fence never closes`)
  }
  const fields: Record<string, string> = {}
  for (const line of lines.slice(1, end)) {
    if (!line.trim()) continue
    const at = line.indexOf(':')
    if (at <= 0) {
      throw new Error(`essay ${slug}: frontmatter line is not 'key: value': ${line.trim()}`)
    }
    fields[line.slice(0, at).trim()] = line
      .slice(at + 1)
      .trim()
      .replace(/^["'](.*)["']$/, '$1')
  }
  return { fields, body: lines.slice(end + 1).join('\n').trim() }
}

/** '<slug>.md' → en; '<slug>.<lang>.md' → that lang. Anything else is a build error. */
function splitName(path: string): { slug: string; lang: Locale } {
  const name = path.split('/').pop()!.replace(/\.md$/, '')
  const dot = name.lastIndexOf('.')
  if (dot === -1) return { slug: name, lang: 'en' }
  const lang = name.slice(dot + 1)
  if (!(LOCALES as readonly string[]).includes(lang) || lang === 'en') {
    throw new Error(`essay ${name}: suffix '.${lang}' is not a translation locale (${LOCALES.filter((l) => l !== 'en').join(', ')})`)
  }
  return { slug: name.slice(0, dot), lang: lang as Locale }
}

async function loadEssay(path: string, raw: string): Promise<Omit<Essay, 'locales'>> {
  const { slug, lang } = splitName(path)
  const { fields, body } = parseFrontmatter(raw, `${slug}.${lang}`)
  for (const key of ['title', 'date', 'description']) {
    if (!fields[key]) throw new Error(`essay ${slug}: frontmatter is missing '${key}'`)
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(fields.date)) {
    throw new Error(`essay ${slug}: date must be ISO YYYY-MM-DD, got '${fields.date}'`)
  }
  if (fields.lang && fields.lang !== lang) {
    throw new Error(`essay ${slug}.${lang}: frontmatter says lang '${fields.lang}' but the file name says '${lang}'`)
  }
  return {
    slug,
    title: fields.title,
    date: fields.date,
    description: fields.description,
    lang,
    html: await renderMarkdown(body),
  }
}

let all: Promise<Essay[]> | undefined

async function loadAll(): Promise<Essay[]> {
  const loaded = await Promise.all(Object.entries(FILES).map(([path, raw]) => loadEssay(path, raw)))
  const bySlug = new Map<string, Locale[]>()
  for (const e of loaded) bySlug.set(e.slug, [...(bySlug.get(e.slug) ?? []), e.lang])
  for (const [slug, langs] of bySlug) {
    if (!langs.includes('en')) {
      throw new Error(`essay ${slug}: a translation exists (${langs.join(', ')}) but no English original ${slug}.md`)
    }
  }
  return loaded.map((e) => ({
    ...e,
    locales: LOCALES.filter((l) => bySlug.get(e.slug)!.includes(l)),
  }))
}

/**
 * The essays in one locale, newest first — the index reads top-down like
 * the changelog. The en list is every essay; a translated locale's list is
 * only the essays that exist in it, so a locale index never links a page
 * that would render in another language.
 */
export async function listEssays(lang: Locale = 'en'): Promise<Essay[]> {
  all ??= loadAll()
  return (await all)
    .filter((e) => e.lang === lang)
    .sort((a, b) => (a.date === b.date ? a.slug.localeCompare(b.slug) : b.date.localeCompare(a.date)))
}

/**
 * The index rows for one locale: every essay, in that locale's copy when
 * one exists and the English original otherwise, so a reader of the ko or
 * ja index sees the whole shelf — the en-only rows link to /essays/<slug>/
 * and the page marks them as English (`fallback`). Newest first.
 */
export async function essayIndex(lang: Locale): Promise<Array<Essay & { fallback: boolean }>> {
  all ??= loadAll()
  const loaded = await all
  const slugs = [...new Set(loaded.map((e) => e.slug))]
  return slugs
    .map((slug) => {
      const own = loaded.find((e) => e.slug === slug && e.lang === lang)
      const en = loaded.find((e) => e.slug === slug && e.lang === 'en')!
      return { ...(own ?? en), fallback: !own }
    })
    .sort((a, b) => (a.date === b.date ? a.slug.localeCompare(b.slug) : b.date.localeCompare(a.date)))
}

/** The essay a locale's landing points at: the newest one written in it. */
export async function latestEssay(lang: Locale): Promise<Essay | undefined> {
  return (await listEssays(lang))[0]
}
