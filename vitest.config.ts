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
          exclude: ['web/src/lib/hosted-fetch.test.ts'],
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
    ],
  },
})
