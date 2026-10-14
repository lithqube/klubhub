/**
 * Browser-only demo (docs/DEMO.md). A no-op unless the app was built with
 * NUXT_DEMO=1. Nuxt excludes this plugin from registration in every normal
 * build, so app/demo and its seeds are absent from the dependency graph.
 * The runtime check only allows a dedicated demo build to disable its API.
 */
interface DemoApi {
  reset: () => void
  sampleTracklist: string
}

export default defineNuxtPlugin({
  name: 'klubhub-demo',
  enforce: 'pre',
  async setup() {
    const config = useRuntimeConfig()
    let demo: DemoApi | null = null
    if (config.public.demo) {
      const { startDemo, SAMPLE_TRACKLIST_TXT } = await import('../demo')
      const backend = startDemo(config.app.baseURL || '/')
      demo = { reset: () => backend.reset(), sampleTracklist: SAMPLE_TRACKLIST_TXT }
    }
    return { provide: { demo } }
  },
})
