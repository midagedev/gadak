import { defineConfig } from 'astro/config'

// gadak.dev apex site. /demo/ and /backlog/ stay owned by the hosted-demo
// build (tools/hosted-demo/build.mjs); this site is the document root around
// them. en is the default locale; ko and ja live under /ko/ and /ja/.
export default defineConfig({
  site: 'https://gadak.dev',
  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'ko', 'ja'],
    routing: { prefixDefaultLocale: false },
  },
})
