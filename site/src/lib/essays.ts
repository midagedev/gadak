import { renderMarkdown } from './changelog'

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
 * Essays are English-only canonical: no ko mirror exists, so their pages
 * pass noAltLang to the layout and the sitemap claims no alternates for
 * them. Adding an essay is dropping a file in the directory — the loader is
 * the registration, and a malformed file fails the build loudly.
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
  /** 'en' unless the file says otherwise. Recorded in JSON-LD only. */
  lang: string
  /** Body, already rendered. */
  html: string
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

async function loadEssay(path: string, raw: string): Promise<Essay> {
  const slug = path.split('/').pop()!.replace(/\.md$/, '')
  const { fields, body } = parseFrontmatter(raw, slug)
  for (const key of ['title', 'date', 'description']) {
    if (!fields[key]) throw new Error(`essay ${slug}: frontmatter is missing '${key}'`)
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(fields.date)) {
    throw new Error(`essay ${slug}: date must be ISO YYYY-MM-DD, got '${fields.date}'`)
  }
  return {
    slug,
    title: fields.title,
    date: fields.date,
    description: fields.description,
    lang: fields.lang || 'en',
    html: await renderMarkdown(body),
  }
}

/** All essays, newest first — the index reads top-down like the changelog. */
export async function listEssays(): Promise<Essay[]> {
  const essays = await Promise.all(
    Object.entries(FILES).map(([path, raw]) => loadEssay(path, raw)),
  )
  return essays.sort((a, b) =>
    a.date === b.date ? a.slug.localeCompare(b.slug) : b.date.localeCompare(a.date),
  )
}
