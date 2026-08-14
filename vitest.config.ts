import { defineConfig } from 'vitest/config'

// Separate from vite.config.ts so the app build keeps root: 'web', the
// svelte/tailwind plugins, and an unset VITE_HOSTED_DEMO. A `test` block
// there would either inherit that root (include paths get confusing) or
// leak the hosted-demo define into `vite build`.
export default defineConfig({
  define: {
    'import.meta.env.VITE_HOSTED_DEMO': JSON.stringify('1'),
  },
  test: {
    environment: 'node',
    include: ['web/src/**/*.test.ts'],
  },
})
