import { test, expect } from '@playwright/test'

/**
 * GDK-975 — /demo/gadak-demo.db is served, and it is a file someone else's
 * SQLite can open.
 *
 * The demo page beside it is frozen JSON: it proves the UI works, and proves
 * nothing about the claim the announcement actually makes — that this is one
 * ordinary SQLite file you can point another tool at. That claim is a URL,
 * so the gate is that the URL answers, not that the file exists in the repo.
 * It existed in the repo the whole time the link was impossible.
 *
 * The build asserts portability too (tools/hosted-demo/build.mjs), but a
 * build-time check cannot see a copy that never reached the artifact, and
 * that is the failure this issue was about.
 */

const DB_PATH = '/demo/gadak-demo.db'

test.describe('GDK-975 the demo mirror is downloadable', () => {
  test('the URL answers with a SQLite file, not an HTML 404 page', async ({ request }) => {
    const res = await request.get(DB_PATH)
    expect(res.status(), `${DB_PATH} must be published in dist/hosted`).toBe(200)
    const body = await res.body()
    // A missing file on a static host is an HTML page with status 200 on some
    // setups, so assert the format, not the status alone.
    expect(
      body.subarray(0, 15).toString('latin1'),
      'not a SQLite 3 database — check what the build copied',
    ).toBe('SQLite format 3')
    // Guard against an empty or truncated copy; the fixture is ~9 MB.
    expect(body.length).toBeGreaterThan(1_000_000)
  })

  test('the published FTS is the portable one, or Datasette Lite cannot open it', async ({
    request,
  }) => {
    const body = await request.get(DB_PATH).then((r) => r.body())
    const text = body.toString('latin1')
    expect(text, 'no items_fts — this is not a gadak mirror').toContain(
      'CREATE VIRTUAL TABLE items_fts',
    )
    // contentless_delete=1 needs SQLite 3.43+; pyodide's is older, so a
    // snapshot carrying it fails to open in the browser and only a human
    // clicking the link would ever find out. Snapshots are rebuilt without
    // it (scripts/scrub-demo-db.py, internal/store/fts_repair.go).
    expect(
      text.includes('contentless_delete'),
      'the published snapshot carries contentless_delete — regenerated from the operational schema?',
    ).toBe(false)
  })
})
