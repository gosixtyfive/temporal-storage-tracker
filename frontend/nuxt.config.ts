// Where /api/** is proxied to. The Go server (cmd/server) listens on :8080 by
// default; override with NUXT_API_TARGET, e.g. NUXT_API_TARGET=http://localhost:18080.
const apiTarget = (process.env.NUXT_API_TARGET ?? 'http://localhost:8080').replace(/\/$/, '')

export default defineNuxtConfig({
  compatibilityDate: '2026-11-01',
  devtools: { enabled: true },

  // Client-side only: the role lives in browser storage, so server rendering
  // would paint the wrong role first. See docs/architecture.md#frontend.
  ssr: false,

  modules: ['@nuxt/ui'],
  css: ['~/assets/css/main.css'],

  app: {
    head: {
      title: 'Storage Tracker',
      meta: [{ name: 'description', content: 'Makerspace storage tracker: a Temporal workflow demo' }],
    },
  },

  routeRules: {
    '/api/**': { proxy: `${apiTarget}/api/**` },
  },
})
