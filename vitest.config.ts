import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vitest/config'

// Separate from vite.config.ts so the app build keeps root: 'web', the
// svelte/tailwind plugins, and an unset VITE_HOSTED_DEMO. A `test` block
// there would either inherit that root (include paths get confusing) or
// leak the hosted-demo define into `vite build`.
//
// Two projects so both compile-time branches of hosted-fetch.ts run.
// The default `unit` project matches a normal `gadak serve` / desktop
// build (flag unset). The hosted-adapter project is the only place the
// flag is '1' — same value tools/hosted-demo/build.mjs sets.
//
// pages-store is a third project: pages.test.ts imports pageAuthorGroupKey
// from pages.svelte.ts, which uses runes and pulls the store graph. The
// default unit project has no svelte plugin on purpose (HistoryView /
// SearchBox tests parse .svelte source with svelte/compiler instead).
// filters-actor.test.ts joins it for the same reason: filterIssues and
// buildGroups live in filters.svelte.ts. docs-empty.test.ts imports the
// docs-empty store, which uses $state in a class.
//
// GDK-1475 measured the alternative — give `unit` the plugin and delete both
// lists. It is not free: four alternating paired runs on this tree put the
// merged config about 7% above the split on CPU time (min 31.3s vs 29.2s,
// every merged sample above every split sample but one), so the split stays.
// The two lists below are hand-kept and must agree: a file dropped from one
// and not added to the other stops being run anywhere, with nothing red to
// say so. e2e/vitest-project-routing.unit.ts is what says so — it asserts
// that every web/src test file is claimed by exactly one project and that no
// list entry names a file that has been renamed away.
//
// Use test.env, not vite `define`. Vitest's deleteDefineConfig copies
// import.meta.env.* defines onto process.env, so a hosted-project define
// would turn the adapter on for the unit project too.
export default defineConfig({
  test: {
    environment: 'node',
    projects: [
      {
        extends: true,
        test: {
          name: 'unit',
          include: ['web/src/**/*.test.ts'],
          exclude: [
            'web/src/lib/hosted-fetch.test.ts',
            'web/src/stores/pages.test.ts',
            'web/src/stores/filters-actor.test.ts',
            'web/src/stores/docs-empty.test.ts',
            // me.feed.test.ts imports the me store (runes + store graph).
            'web/src/stores/me.feed.test.ts',
            // format.ts routes status ink through the ui-tokens runes store,
            // so these suites need the svelte plugin too (GDK-786).
            'web/src/lib/format.test.ts',
            'web/src/stores/ui-tokens.test.ts',
          ],
          // Pin empty so a leftover VITE_HOSTED_DEMO=1 in the shell cannot
          // turn the adapter on for the production-default suite.
          env: { VITE_HOSTED_DEMO: '' },
        },
      },
      {
        extends: true,
        test: {
          name: 'hosted-adapter',
          include: ['web/src/lib/hosted-fetch.test.ts'],
          env: { VITE_HOSTED_DEMO: '1' },
        },
      },
      {
        plugins: [svelte()],
        test: {
          name: 'pages-store',
          environment: 'node',
          include: [
            'web/src/stores/pages.test.ts',
            'web/src/stores/filters-actor.test.ts',
            'web/src/stores/docs-empty.test.ts',
            'web/src/stores/me.feed.test.ts',
            'web/src/lib/format.test.ts',
            'web/src/stores/ui-tokens.test.ts',
          ],
          env: { VITE_HOSTED_DEMO: '' },
        },
      },
      // Node-only e2e identity guard (no svelte, no browser). Named
      // *.unit.ts so Playwright's *.{test,spec}.ts matcher does not
      // pick these up as browser specs.
      {
        extends: true,
        test: {
          name: 'e2e-guard',
          include: ['e2e/**/*.unit.ts'],
          env: { VITE_HOSTED_DEMO: '' },
        },
      },
    ],
  },
})
