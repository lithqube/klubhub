import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Venue } from '../types/gig'

export const useVenueStore = defineStore('venue', () => {
  const venues = ref<Venue[]>([])

  async function fetchAutocomplete(
    q: string,
    limit = 5
  ): Promise<Venue[]> {
    try {
      const data = await $fetch<Venue[]>(
        `/api/v1/venues/autocomplete?q=${encodeURIComponent(q)}&limit=${limit}`
      )
      venues.value = Array.isArray(data) ? data : (data as any).data || []
      return venues.value
    } catch (e) {
      console.error('fetchAutocomplete failed:', e)
      return []
    }
  }

  return { venues, fetchAutocomplete }
})
