import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Gig, GigCreate, GigUpdate, GigFilter } from '../types/gig'
import { GigConflictError } from '../types/gig'

// Group B.4 (v1-connect-product) standardized gig list/get/create/update
// responses on the `{ data }` envelope used by the social & finance
// handlers. Some endpoints (autocomplete) still return the bare shape;
// this helper accepts both so the store keeps working if the API
// regresses.
function unwrap<T>(result: T | { data: T } | null | undefined): T | null {
  if (result == null) return null
  if (typeof result === 'object' && 'data' in (result as object)) {
    return (result as { data: T }).data ?? null
  }
  return result as T
}

// Unwrap an envelope around an array; returns [] when the response is
// missing, not an object, or its `data` field is null.
function unwrapList<T>(result: T[] | { data: T[] | null } | null | undefined): T[] {
  const inner = unwrap(result)
  return Array.isArray(inner) ? inner : []
}

// fee_amount is a decimal on the API and arrives as a string ("1500").
function feeOf(g: Gig): number {
  return Number(g.fee_amount) || 0
}

export const useGigStore = defineStore('gig', () => {
  const gigs = ref<Gig[]>([])
  // Unfiltered dashboard results must never replace the gigs-page filter cache.
  const dashboardGigs = ref<Gig[]>([])
  const loading = ref(false)
  // Per-record request generations: a later operation owns cache installation.
  // Unrelated gig requests cannot invalidate one another.
  const generations = new Map<string, number>()
  // Record ownership and list membership are separate. List installations
  // invalidate old GET/PUT responses, but cannot pin another view's membership.
  const recordOperations = new Map<string, number>()
  const snapshots = new Map<string, Gig>()
  const pendingDeletes = new Map<string, number>()
  function beginOperation(id: string, recordOperation = true): number {
    const generation = (generations.get(id) ?? 0) + 1
    generations.set(id, generation)
    if (recordOperation) recordOperations.set(id, generation)
    return generation
  }
  function installSnapshot(gig: Gig): void {
    snapshots.set(gig.id, gig)
    // Refresh fields only for existing members; never copy list membership.
    for (const rows of [gigs.value, dashboardGigs.value]) {
      const idx = rows.findIndex(row => row.id === gig.id)
      if (idx !== -1) rows[idx] = gig
    }
  }
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
    return playedGigs.reduce((sum, g) => sum + feeOf(g), 0)
  })

  const avgFee = computed(() => {
    const playedGigs = gigs.value.filter((g) => g.status === 'played')
    if (playedGigs.length === 0) return 0
    const total = playedGigs.reduce((sum, g) => sum + feeOf(g), 0)
    return Math.round(total / playedGigs.length)
  })

  // Both list consumers share record generations, including retained deletion
  // tombstones. Capture at request start, not by comparing cache values (ABA).
  function beginListRead() {
    const started = new Map(generations)
    const operationsAtStart = new Map(recordOperations)
    const deletingAtStart = new Set(pendingDeletes.keys())
    const recordChangedSinceStart = (id: string) => deletingAtStart.has(id) || pendingDeletes.has(id)
      || (recordOperations.get(id) ?? 0) !== (operationsAtStart.get(id) ?? 0)
    const changedSinceStart = (id: string) => recordChangedSinceStart(id)
      || (generations.get(id) ?? 0) !== (started.get(id) ?? 0)
    function install(rows: Gig[], current: Gig[]): Gig[] {
      const previous = new Map(current.map(g => [g.id, g]))
      const next: Gig[] = []
      for (const gig of rows) {
        if (changedSinceStart(gig.id)) {
          // The other view may own the snapshot even if this cache was empty.
          // Confirmed deletion clears it, while retaining its generation.
          const remembered = snapshots.get(gig.id) ?? previous.get(gig.id)
          if (remembered) next.push(remembered)
        } else {
          // An authoritative list read also invalidates earlier PUT responses.
          beginOperation(gig.id, false)
          installSnapshot(gig)
          next.push(gig)
        }
        previous.delete(gig.id)
      }
      for (const gig of previous.values()) {
        // Only record operations preserve omitted members (e.g. pending DELETE
        // or a newly saved gig), never another view's list installation.
        if (recordChangedSinceStart(gig.id)) next.push(snapshots.get(gig.id) ?? gig)
        else beginOperation(gig.id, false)
      }
      return next
    }
    return { install, changedSinceStart }
  }

  let listGeneration = 0
  async function fetchGigs(): Promise<void> {
    const listRequest = ++listGeneration
    const { install, changedSinceStart } = beginListRead()
    loading.value = true
    try {
      const params = new URLSearchParams()
      Object.entries(filters.value).forEach(([k, v]) => {
        if (v !== '' && v !== null && v !== undefined) {
          params.append(k, String(v))
        }
      })
      const queryStr = params.toString()
      // Go API returns `{ data: Gig[] }` since B.4; the helper also
      // accepts a bare array for safety against older deployments.
      const result = await $fetch<Gig[] | { data: Gig[] | null }>(
        `/api/v1/gigs${queryStr ? `?${queryStr}` : ''}`
      )
      if (listRequest !== listGeneration) return
      gigs.value = install(unwrapList(result), gigs.value)
    } catch (e) {
      console.error('fetchGigs failed:', e)
      if (listRequest === listGeneration) {
        gigs.value = gigs.value.filter(g => changedSinceStart(g.id))
      }
    } finally {
      if (listRequest === listGeneration) loading.value = false
    }
  }

  let dashboardListGeneration = 0
  async function fetchDashboardGigs(isCurrent: () => boolean): Promise<void> {
    const request = ++dashboardListGeneration
    const { install } = beginListRead()
    const result = await $fetch<Gig[] | { data: Gig[] | null }>('/api/v1/gigs')
    // Unmount invalidates only this read, never another page's list/GET/PUT
    // or shared loading state. Propagate failures for honest local UI errors.
    if (!isCurrent() || request !== dashboardListGeneration) return
    const rows = unwrap(result) ?? []
    if (!Array.isArray(rows)) throw new Error('Invalid list response')
    dashboardGigs.value = install(rows, dashboardGigs.value)
  }

  async function fetchGigDetail(id: string): Promise<Gig | null> {
    try {
      const result = await $fetch<Gig | { data: Gig }>(`/api/v1/gigs/${id}/detail`)
      return unwrap(result)
    } catch (e) {
      console.error('fetchGigDetail failed:', e)
      return null
    }
  }

  async function createGig(gig: GigCreate): Promise<Gig | null> {
    try {
      const result = await $fetch<Gig | { data: Gig }>('/api/v1/gigs', {
        method: 'POST',
        body: gig,
      })
      const created = unwrap(result)
      if (created) {
        beginOperation(created.id)
        installSnapshot(created)
        gigs.value.unshift(created)
        dashboardGigs.value.unshift(created)
      }
      return created
    } catch (e) {
      console.error('createGig failed:', e)
      return null
    }
  }

  function rememberGig(gig: Gig): void {
    // Also mark installation: a list started during this request may contain
    // an older snapshot even if its response arrives after this one.
    beginOperation(gig.id)
    // PUT returns the base gig, whereas GET includes C2 relationships.
    const previous = snapshots.get(gig.id)
      ?? gigs.value.find(row => row.id === gig.id)
      ?? dashboardGigs.value.find(row => row.id === gig.id)
    const snapshot = { ...previous, ...gig }
    installSnapshot(snapshot)
    for (const rows of [gigs.value, dashboardGigs.value]) {
      if (!rows.some(row => row.id === gig.id)) rows.push(snapshot)
    }
  }

  async function fetchGig(id: string): Promise<Gig | null> {
    const generation = beginOperation(id)
    try {
      const result = await $fetch<Gig | { data: Gig }>(`/api/v1/gigs/${id}`)
      const refreshed = unwrap(result)
      if (generations.get(id) !== generation) return null
      if (refreshed) rememberGig(refreshed)
      return refreshed
    } catch (e) {
      console.error('fetchGig failed:', e)
      return null
    }
  }

  async function updateGig(id: string, gig: GigUpdate): Promise<Gig | null> {
    // The caller's fields and version are one snapshot. Never borrow a cache
    // token to authorize stale fields; unknown versions remain null for 409.
    const updatedAt = gig.updated_at ?? null
    const generation = beginOperation(id)
    try {
      const result = await $fetch<Gig | { data: Gig }>(`/api/v1/gigs/${id}`, {
        method: 'PUT',
        body: { ...gig, updated_at: updatedAt },
      })
      const updated = unwrap(result)
      // A stale response is not a usable new edit snapshot either.
      if (generations.get(id) !== generation) return null
      if (updated) rememberGig(updated)
      return updated
    } catch (e) {
      console.error('updateGig failed:', e)
      // An obsolete conflict must not start a GET that invalidates a newer PUT.
      if (generations.get(id) !== generation) return null
      const error = e as { response?: { status?: number }; statusCode?: number; status?: number }
      if ((error?.response?.status ?? error?.statusCode ?? error?.status) === 409) {
        throw new GigConflictError(await fetchGig(id))
      }
      return null
    }
  }

  async function deleteGig(id: string): Promise<boolean> {
    // DELETE owns this record both at start and at confirmation. Keep the
    // generation after removal: reads/PUTs/lists started during DELETE must
    // not reinstall it, even though it is no longer present in the cache.
    beginOperation(id)
    pendingDeletes.set(id, (pendingDeletes.get(id) ?? 0) + 1)
    try {
      await $fetch(`/api/v1/gigs/${id}`, { method: 'DELETE' })
      beginOperation(id)
      snapshots.delete(id)
      gigs.value = gigs.value.filter((g) => g.id !== id)
      dashboardGigs.value = dashboardGigs.value.filter((g) => g.id !== id)
      return true
    } catch (e) {
      console.error('deleteGig failed:', e)
      return false
    } finally {
      const remaining = (pendingDeletes.get(id) ?? 1) - 1
      if (remaining) pendingDeletes.set(id, remaining)
      else pendingDeletes.delete(id)
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
      // Autocomplete endpoint is not yet envelope-wrapped, but tolerate
      // `{ data }` for forward compatibility with B.4-aligned handlers.
      const result = await $fetch<Gig[] | { data: Gig[] | null }>(
        `/api/v1/gigs/autocomplete?q=${encodeURIComponent(q)}&limit=5`
      )
      return unwrapList(result)
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
    dashboardGigs,
    fetchDashboardGigs,
    loading,
    filters,
    upcomingGigs,
    ytdEarned,
    avgFee,
    fetchGigs,
    fetchGigDetail,
    fetchGig,
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
