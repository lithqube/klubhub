import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Tracklist, Track, ParseWarning } from '../types/tracklist'

export const useTracklistStore = defineStore('tracklist', () => {
  const tracklist = ref<Tracklist | null>(null)
  const tracks = ref<Track[]>([])
  const warnings = ref<ParseWarning[]>([])
  const uploadFile = ref<File | null>(null)
  const uploadFilename = ref<string>('')
  const uploadFileSize = ref<string>('')
  const pastTracklists = ref<Tracklist[]>([])

  const abortController = ref<AbortController | null>(null)
  const linkedGigs = ref<Record<string, string[]>>({})

  const trackCount = computed(() => tracks.value.length)

  async function fetchTracklist(id: string): Promise<Tracklist | null> {
    try {
      const result = await $fetch<{ data: Tracklist }>(`/api/v1/tracklists/${id}`)
      return result.data || null
    } catch (e) {
      console.error('fetchTracklist failed:', e)
      return null
    }
  }

  async function loadPastTracklists(): Promise<void> {
    try {
      const result = await $fetch<{ data: Tracklist[] }>('/api/v1/tracklists')
      pastTracklists.value = result.data || []
    } catch (e) {
      console.error('loadPastTracklists failed:', e)
      pastTracklists.value = []
    }
  }

  function reset(): void {
    tracklist.value = null
    tracks.value = []
    warnings.value = []
    uploadFile.value = null
    uploadFilename.value = ''
    uploadFileSize.value = ''
    pastTracklists.value = []
    abortController.value = null
  }

  return {
    tracklist,
    tracks,
    warnings,
    uploadFile,
    uploadFilename,
    uploadFileSize,
    pastTracklists,
    trackCount,
    linkedGigs,
    fetchTracklist,
    loadPastTracklists,
    reset,
  }
})
