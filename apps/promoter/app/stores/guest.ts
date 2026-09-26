import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  AddGuestsInput, AddResult, Allocation, AllocationInput, BulkResult, BulkStatusInput, Guest, GuestCounts, GuestFilter, GuestInput,
  GuestList, GuestPage, ListInput, OverviewRow, StandingList,
} from '~/types/guest'
import { apiFetch, toApiError } from '~/utils/api'

const emptyCounts = (): GuestCounts => ({ all: 0, going: 0, pending: 0, waitlist: 0, invited: 0, declined: 0, going_heads: 0 })

/**
 * Guest lists, allocations and the guest table of one event at a time,
 * plus standing lists and the cross-event overview (P2.1).
 */
export const useGuestStore = defineStore('guest', () => {
  const eventId = ref<string | null>(null)
  const lists = ref<GuestList[]>([])
  const guests = ref<Guest[]>([])
  const counts = ref<GuestCounts>(emptyCounts())
  const standing = ref<StandingList[]>([])
  const overview = ref<OverviewRow[]>([])
  const loading = ref(false)
  const error = ref<ReturnType<typeof toApiError> | null>(null)

  async function call<T>(fn: () => Promise<T>): Promise<T> {
    try {
      return await fn()
    } catch (e) {
      throw toApiError(e)
    }
  }

  const base = (id: string) => `/api/v1/events/${id}`

  /** Load lists and the full guest table of an event (filters apply client-side). */
  async function load(id: string): Promise<void> {
    eventId.value = id
    loading.value = true
    error.value = null
    try {
      const [l, p] = await Promise.all([
        apiFetch<GuestList[]>(`${base(id)}/lists`),
        apiFetch<GuestPage>(`${base(id)}/guests`),
      ])
      lists.value = l
      guests.value = p.guests
      counts.value = p.counts
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  async function refreshLists(): Promise<void> {
    if (eventId.value) lists.value = await call(() => apiFetch<GuestList[]>(`${base(eventId.value!)}/lists`))
  }

  async function refreshGuests(filter: GuestFilter = {}): Promise<void> {
    if (!eventId.value) return
    const p = await call(() => apiFetch<GuestPage>(`${base(eventId.value!)}/guests`, { query: filter }))
    guests.value = p.guests
    counts.value = p.counts
  }

  /** After a guest change: guests and list/allocation fill both move. */
  const refresh = () => Promise.all([refreshGuests(), refreshLists()])

  async function createList(input: ListInput): Promise<GuestList> {
    const l = await call(() => apiFetch<GuestList>(`${base(eventId.value!)}/lists`, { method: 'POST', body: input }))
    lists.value = [...lists.value, l]
    return l
  }

  async function updateList(listId: string, input: ListInput): Promise<GuestList> {
    const l = await call(() => apiFetch<GuestList>(`${base(eventId.value!)}/lists/${listId}`, { method: 'PUT', body: input }))
    lists.value = lists.value.map(x => (x.id === l.id ? l : x))
    await refreshGuests()
    return l
  }

  async function deleteList(listId: string, force = false): Promise<void> {
    await call(() => apiFetch(`${base(eventId.value!)}/lists/${listId}`, { method: 'DELETE', query: force ? { force: 'true' } : undefined }))
    lists.value = lists.value.filter(x => x.id !== listId)
    if (force) await refreshGuests()
  }

  async function createAllocation(listId: string, input: AllocationInput): Promise<Allocation> {
    const a = await call(() => apiFetch<Allocation>(`${base(eventId.value!)}/lists/${listId}/allocations`, { method: 'POST', body: input }))
    await refreshLists()
    return a
  }

  async function updateAllocation(listId: string, id: string, input: AllocationInput): Promise<Allocation> {
    const a = await call(() => apiFetch<Allocation>(`${base(eventId.value!)}/lists/${listId}/allocations/${id}`, { method: 'PUT', body: input }))
    await refreshLists()
    return a
  }

  async function revokeAllocation(listId: string, id: string): Promise<void> {
    await call(() => apiFetch(`${base(eventId.value!)}/lists/${listId}/allocations/${id}`, { method: 'DELETE' }))
    await refreshLists()
  }

  async function addGuests(input: AddGuestsInput): Promise<AddResult> {
    const r = await call(() => apiFetch<AddResult>(`${base(eventId.value!)}/guests`, { method: 'POST', body: input }))
    await refresh()
    return r
  }

  async function updateGuest(id: string, input: GuestInput): Promise<Guest> {
    const g = await call(() => apiFetch<Guest>(`${base(eventId.value!)}/guests/${id}`, { method: 'PUT', body: input }))
    await refresh()
    return g
  }

  async function deleteGuest(id: string): Promise<void> {
    await call(() => apiFetch(`${base(eventId.value!)}/guests/${id}`, { method: 'DELETE' }))
    await refresh()
  }

  async function bulkStatus(input: BulkStatusInput): Promise<BulkResult> {
    const r = await call(() => apiFetch<BulkResult>(`${base(eventId.value!)}/guests/bulk-status`, { method: 'POST', body: input }))
    await refresh()
    return r
  }

  /** CSV text from the server (injection-safe, audited). */
  function exportCsv(filter: GuestFilter = {}): Promise<string> {
    return call(() => apiFetch<string>(`${base(eventId.value!)}/guests/export.csv`, { query: filter, responseType: 'text' }))
  }

  async function fetchStanding(): Promise<void> {
    standing.value = await call(() => apiFetch<StandingList[]>('/api/v1/standing-lists'))
  }

  async function saveStanding(input: ListInput, id?: string): Promise<StandingList> {
    const s = await call(() => id
      ? apiFetch<StandingList>(`/api/v1/standing-lists/${id}`, { method: 'PUT', body: input })
      : apiFetch<StandingList>('/api/v1/standing-lists', { method: 'POST', body: input }))
    standing.value = id ? standing.value.map(x => (x.id === id ? s : x)) : [...standing.value, s]
    return s
  }

  async function deleteStanding(id: string): Promise<void> {
    await call(() => apiFetch(`/api/v1/standing-lists/${id}`, { method: 'DELETE' }))
    standing.value = standing.value.filter(x => x.id !== id)
  }

  async function fetchOverview(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      overview.value = await apiFetch<OverviewRow[]>('/api/v1/guests/overview')
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  return {
    eventId, lists, guests, counts, standing, overview, loading, error,
    load, refreshLists, refreshGuests, createList, updateList, deleteList, createAllocation, updateAllocation, revokeAllocation,
    addGuests, updateGuest, deleteGuest, bulkStatus, exportCsv, fetchStanding, saveStanding, deleteStanding, fetchOverview,
  }
})
