import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { RAArtist, RAEVENT, RAImportResult, RAImportRequest } from '../types/ra'
import { useFeatures } from '../composables/useFeatures'

// Prefer the API's own explanation ({ "error": "..." }) over ofetch's generic
// `[POST] "/api/...": 502 Bad Gateway`, which tells the user nothing.
function messageOf(e: unknown, fallback: string): string {
  const body = (e as { data?: { error?: string; message?: string } } | null)?.data
  if (body?.error) return body.error
  if (body?.message) return body.message
  return e instanceof Error ? e.message : fallback
}

// RA import is a licensed edition feature (docs/EDITIONS.md). When it is
// off, every action below is a no-op that never calls the API, so nothing
// can reach the (unmounted) RA endpoints even if a component slipped
// through the UI gating.
const DISABLED_MESSAGE = 'This feature is not available in this edition.'

export const useRaStore = defineStore('ra', () => {
  const { isEnabled } = useFeatures()
  const enabled = isEnabled('raImport')

  // State
  const artist = ref<RAArtist | null>(null)
  const events = ref<RAEVENT[]>([])
  const importResult = ref<RAImportResult | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Actions
  async function fetchArtist(slug: string): Promise<void> {
    if (!enabled) return
    loading.value = true
    error.value = null
    try {
      const result = await $fetch<{ data: RAArtist }>(`/api/v1/gigs/info/${slug}`)
      artist.value = result.data
    } catch (e) {
      error.value = messageOf(e, 'Failed to fetch artist')
      artist.value = null
    } finally {
      loading.value = false
    }
  }

  async function fetchEvents(slug: string): Promise<void> {
    if (!enabled) return
    loading.value = true
    error.value = null
    try {
      // Fetch artist first to get the events
      await fetchArtist(slug)
      // For now, events will be fetched via the import endpoint
      // The backend handles event fetching internally
    } catch (e) {
      error.value = messageOf(e, 'Failed to fetch events')
    } finally {
      loading.value = false
    }
  }

  async function importEvents(request: RAImportRequest): Promise<RAImportResult> {
    if (!enabled) throw new Error(DISABLED_MESSAGE)
    loading.value = true
    error.value = null
    try {
      const result = await $fetch<RAImportResult>('/api/v1/gigs/import-ra', {
        method: 'POST',
        body: request,
      })
      importResult.value = result
      return result
    } catch (e) {
      error.value = messageOf(e, 'Import failed')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function importFromEpk(request: RAImportRequest): Promise<RAImportResult> {
    if (!enabled) throw new Error(DISABLED_MESSAGE)
    loading.value = true
    error.value = null
    try {
      const result = await $fetch<RAImportResult>('/api/v1/epk/import-ra', {
        method: 'POST',
        body: request,
      })
      error.value = null
      return result
    } catch (e) {
      error.value = messageOf(e, 'EPK import failed')
      throw e
    } finally {
      loading.value = false
    }
  }

  function clear() {
    artist.value = null
    events.value = []
    importResult.value = null
    error.value = null
  }

  return {
    enabled,
    artist,
    events,
    importResult,
    loading,
    error,
    fetchArtist,
    fetchEvents,
    importEvents,
    importFromEpk,
    clear,
  }
})
