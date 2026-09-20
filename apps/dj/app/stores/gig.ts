import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Gig, GigCreate, GigUpdate, GigFilter } from '../types/gig'

export const useGigStore = defineStore('gig', () => {
  const gigs = ref<Gig[]>([])
  const loading = ref(false)
  const filters = ref<GigFilter>({
    status: '',
    venue: '',
    city: '',
    fee_min: null as number | null,
    fee_max: null as number | null,
    from: '',
    to: '',
  })

  const upcomingGigs = computed(() =>
    gigs.value.filter((g) => !['played', 'cancelled'].includes(g.status))
  )

  const currentYear = new Date().getFullYear()
  const ytdEarned = computed(() => {
    const playedGigs = gigs.value.filter(
      (g) => g.status === 'played' && new Date(g.date).getFullYear() === currentYear
    )
    if (playedGigs.length === 0) return 0
    return playedGigs.reduce((sum, g) => sum + (g.fee_amount || 0), 0)
  })

  const avgFee = computed(() => {
    const playedGigs = gigs.value.filter((g) => g.status === 'played')
    if (playedGigs.length === 0) return 0
    const total = playedGigs.reduce((sum, g) => sum + (g.fee_amount || 0), 0)
    return Math.round(total / playedGigs.length)
  })

  async function fetchGigs(): Promise<void> {
    loading.value = true
    try {
      const params = new URLSearchParams()
      Object.entries(filters.value).forEach(([k, v]) => {
        if (v !== '' && v !== null && v !== undefined) {
          params.append(k, String(v))
        }
      })
      const queryStr = params.toString()
      const data = await $fetch<Gig[]>(
        `/api/v1/gigs${queryStr ? `?${queryStr}` : ''}`
      )
      gigs.value = Array.isArray(data) ? data : (data as any).data || []
    } catch (e) {
      console.error('fetchGigs failed:', e)
      gigs.value = []
    } finally {
      loading.value = false
    }
  }

  async function fetchGigDetail(id: string): Promise<Gig | null> {
    try {
      const result = await $fetch<{ data: Gig }>(`/api/v1/gigs/${id}/detail`)
      return result.data
    } catch (e) {
      console.error('fetchGigDetail failed:', e)
      return null
    }
  }

  async function createGig(gig: GigCreate): Promise<Gig | null> {
    try {
      const result = await $fetch<{ data: Gig }>('/api/v1/gigs', {
        method: 'POST',
        body: gig,
      })
      const created = result.data
      gigs.value.unshift(created)
      return created
    } catch (e) {
      console.error('createGig failed:', e)
      return null
    }
  }

  async function updateGig(id: string, gig: GigUpdate): Promise<Gig | null> {
    try {
      const result = await $fetch<{ data: Gig }>(`/api/v1/gigs/${id}`, {
        method: 'PUT',
        body: gig,
      })
      const updated = result.data
      const idx = gigs.value.findIndex((g) => g.id === id)
      if (idx !== -1) {
        gigs.value[idx] = updated
      }
      return updated
    } catch (e) {
      console.error('updateGig failed:', e)
      return null
    }
  }

  async function deleteGig(id: string): Promise<boolean> {
    try {
      await $fetch(`/api/v1/gigs/${id}`, { method: 'DELETE' })
      gigs.value = gigs.value.filter((g) => g.id !== id)
      return true
    } catch (e) {
      console.error('deleteGig failed:', e)
      return false
    }
  }

  async function linkVenue(
    gigId: string,
    venueId: string,
    isPrimary: boolean
  ): Promise<boolean> {
    try {
      await $fetch(`/api/v1/gigs/${gigId}/link-venue`, {
        method: 'POST',
        body: { venue_id: venueId, is_primary: isPrimary },
      })
      return true
    } catch (e) {
      console.error('linkVenue failed:', e)
      return false
    }
  }

  async function linkContact(
    gigId: string,
    contactId: string,
    role: string
  ): Promise<boolean> {
    try {
      await $fetch(`/api/v1/gigs/${gigId}/link-contact`, {
        method: 'POST',
        body: { contact_id: contactId, role },
      })
      return true
    } catch (e) {
      console.error('linkContact failed:', e)
      return false
    }
  }

  async function linkTracklist(
    gigId: string,
    tracklistId: string
  ): Promise<boolean> {
    try {
      await $fetch(`/api/v1/gigs/${gigId}/tracklists/${tracklistId}`, {
        method: 'POST',
      })
      return true
    } catch (e) {
      console.error('linkTracklist failed:', e)
      return false
    }
  }

  async function unlinkTracklist(
    gigId: string,
    tracklistId: string
  ): Promise<boolean> {
    try {
      await $fetch(`/api/v1/gigs/${gigId}/tracklists/${tracklistId}`, {
        method: 'DELETE',
      })
      return true
    } catch (e) {
      console.error('unlinkTracklist failed:', e)
      return false
    }
  }
  async function fetchGigsAutocomplete(q: string): Promise<Gig[]> {
    try {
      const data = await $fetch<Gig[]>(
        `/api/v1/gigs/autocomplete?q=${encodeURIComponent(q)}&limit=5`
      )
      return Array.isArray(data) ? data : []
    } catch (e) {
      console.error('fetchGigsAutocomplete failed:', e)
      return []
    }
  }

  function generateICalUrl(): string {
    const secret = useRuntimeConfig().public.icalSecret || ''
    return `/api/v1/gigs/calendar.ics?secret=${secret}`
  }

  function setFilter(key: keyof GigFilter, value: string | number | null): void {
    ;(filters.value as any)[key] = value
  }

  function clearFilters(): void {
    filters.value = {
      status: '',
      venue: '',
      city: '',
      fee_min: null,
      fee_max: null,
      from: '',
      to: '',
    }
  }

  return {
    gigs,
    loading,
    filters,
    upcomingGigs,
    ytdEarned,
    avgFee,
    fetchGigs,
    fetchGigDetail,
    createGig,
    updateGig,
    deleteGig,
    linkVenue,
    linkContact,
    linkTracklist,
    unlinkTracklist,
    fetchGigsAutocomplete,
    generateICalUrl,
    setFilter,
    clearFilters,
  };
})
