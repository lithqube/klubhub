import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ApiError, Venue, VenueInput, VenueProtected } from '~/types/event'
import { apiFetch, toApiError } from '~/utils/api'

export const useVenueStore = defineStore('venue', () => {
  const venues = ref<Venue[]>([])
  const loading = ref(false)
  const error = ref<ApiError | null>(null)

  async function fetchVenues(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      venues.value = await apiFetch<Venue[]>('/api/v1/venues')
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  async function getVenue(id: string): Promise<Venue> {
    return apiFetch<Venue>(`/api/v1/venues/${id}`)
  }

  /** Returns the saved venue, or throws an ApiError. */
  async function saveVenue(input: VenueInput, id?: string): Promise<Venue> {
    try {
      const v = id
        ? await apiFetch<Venue>(`/api/v1/venues/${id}`, { method: 'PUT', body: input })
        : await apiFetch<Venue>('/api/v1/venues', { method: 'POST', body: input })
      const i = venues.value.findIndex(x => x.id === v.id)
      if (i >= 0) venues.value[i] = v
      else venues.value = [...venues.value, v].sort((a, b) => a.name.localeCompare(b.name))
      return v
    } catch (e) {
      throw toApiError(e)
    }
  }

  async function archiveVenue(id: string): Promise<void> {
    try {
      await apiFetch(`/api/v1/venues/${id}`, { method: 'DELETE' })
      venues.value = venues.value.filter(v => v.id !== id)
    } catch (e) {
      throw toApiError(e)
    }
  }

  /** Decrypts protected fields; audited server-side, needs a second factor. */
  async function revealVenue(id: string): Promise<VenueProtected> {
    try {
      return await apiFetch<VenueProtected>(`/api/v1/venues/${id}/reveal`, { method: 'POST' })
    } catch (e) {
      throw toApiError(e)
    }
  }

  return { venues, loading, error, fetchVenues, getVenue, saveVenue, archiveVenue, revealVenue }
})
