// Entry point for the browser-only demo build (NUXT_DEMO=1, docs/DEMO.md).

import { setApiAssetResolver } from '../utils/apiAssetUrl'
import { createDemoBackend, type DemoBackend } from './backend'
import { installDemoFetch } from './fetch'
import { browserStorage } from './state'

export type { DemoBackend }
export { SAMPLE_TRACKLIST_TXT } from './seed/tracklists'

/** Installs the in-browser API and returns the backend (for reset). */
export function startDemo(baseURL: string): DemoBackend {
  const backend = createDemoBackend({ storage: browserStorage(), baseURL })
  installDemoFetch(backend, { baseURL, latencyMs: 60 })
  setApiAssetResolver((path) => backend.resolveAsset(path))
  return backend
}
