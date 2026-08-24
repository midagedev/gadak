// GDK-800 1차 DOM smoke: load the vite dev bundle in a real browser engine,
// point it at a live `gadak demo` serve (7899 — deliberately NOT the e2e port
// 7877), and verify (1) real queue rows render from real data, (2) orca #5049
// — network failure keeps the last-good rows and shows a banner instead of a
// blank screen, (3) the never-loaded case shows the pairing CTA, not a void.
//
// Playwright is imported from the repo root's node_modules by absolute path —
// it is NOT a mobile dependency (only @tauri-apps/*, vite/svelte, vitest are
// allowed) and the root install is shared infrastructure, not modified.
//
// Prereqs, both must already be running:
//   gadak demo --addr 127.0.0.1:7899 --no-open   (from the repo root)
//   npm run dev                                    (vite on 5180)
import { createRequire } from 'node:module';

const ROOT = new URL('../../', import.meta.url).pathname; // repo root
const require = createRequire(ROOT + 'package.json');
const { chromium } = require('playwright');

const APP = 'http://localhost:5180';
const results = [];
const note = (name, ok, detail = '') => {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' — ' + detail : ''}`);
};

const browser = await chromium.launch();
try {
  // --- Phase 1: online, real data ------------------------------------
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  const netlog = [];
  page.on('request', (r) => {
    const u = new URL(r.url());
    if (u.origin !== new URL(APP).origin) netlog.push(u.origin);
  });
  await page.goto(APP, { waitUntil: 'networkidle' });

  const rows = page.locator('[data-testid="queue-row"]');
  await rows.first().waitFor({ state: 'visible', timeout: 15000 });
  const rowCount = await rows.count();
  note('queue rows from real serve data', rowCount > 0, `${rowCount} rows`);

  const banner = page.locator('[data-testid="offline-banner"]');
  note('no offline banner while online', (await banner.count()) === 0);

  const sample = await rows.first().evaluate((el) => ({
    key: el.dataset.key,
    text: el.innerText.replace(/\s+/g, ' ').slice(0, 140),
  }));
  note('first row has key + text', Boolean(sample.key) && sample.text.length > 0,
    JSON.stringify(sample));

  // Outbound discipline: every request stayed on the vite origin.
  note('no off-origin requests', netlog.length === 0, netlog.join(', ') || 'none');

  const domText = await page.locator('.m-rows').innerText();
  console.log('\n--- DOM text dump (first 1200 chars of .m-rows) ---');
  console.log(domText.slice(0, 1200));
  console.log('--- end dump ---\n');

  // --- Phase 2: orca #5049 — fail, keep last-good ---------------------
  await page.route('**/api/**', (route) => route.abort());
  await page.locator('[data-testid="refresh"]').click();
  await banner.waitFor({ state: 'visible', timeout: 15000 });
  note('offline banner appears on failure', true);
  note('rows retained after failure', (await rows.count()) === rowCount,
    `${await rows.count()} rows`);
  note('banner says last-sync (ever loaded)',
    await banner.getAttribute('data-ever-loaded') === '1');

  // --- Phase 3: never loaded — CTA, not blank ------------------------
  const fresh = await browser.newPage({ viewport: { width: 390, height: 844 } });
  await fresh.route('**/api/**', (route) => route.abort());
  await fresh.goto(APP, { waitUntil: 'networkidle' });
  await fresh.locator('[data-testid="offline-banner"]').waitFor({ state: 'visible', timeout: 15000 });
  const cta = fresh.locator('[data-testid="queue-pair-cta"]');
  note('fresh+offline shows pairing CTA', (await cta.count()) === 1);
  note('fresh+offline shows zero rows', (await fresh.locator('[data-testid="queue-row"]').count()) === 0);
  await fresh.close();

  await page.close();
} finally {
  await browser.close();
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} checks passed`);
if (failed.length) {
  console.error('SMOKE FAILED');
  process.exit(1);
}
console.log('SMOKE PASSED');
