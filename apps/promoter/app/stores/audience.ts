import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  Contact, ContactInput, Counts, ImportResult, ImportRow, Segment, SegmentInput, StatusInput as AudienceStatusInput,
} from '~/types/audience'
import { apiFetch, toApiError } from '~/utils/api'

interface Filter { status?: string, source?: string, q?: string }

const emptyCounts = (): Counts => ({ all: 0, active: 0, unsubscribed: 0, bounced: 0, complained: 0 })

/**
 * The audience CRM: contacts with a consent record and saved-filter
 * segments (P3.1). Read-only per the D2 self-hosted boundary — nothing
 * here runs a public follow/notify-me form; contacts arrive through the
 * admin UI or a CSV import.
 */
export const useAudienceStore = defineStore('audience', () => {
  const contacts = ref<Contact[]>([])
  const counts = ref<Counts>(emptyCounts())
  const segments = ref<Segment[]>([])
  const loading = ref(false)
  const error = ref<ReturnType<typeof toApiError> | null>(null)
  // Remembered so a reload after setStatus/importCSV can reapply whatever
  // status/source/search the page had selected, instead of silently
  // showing everyone while the filter controls still say otherwise.
  let lastFilter: Filter = {}

  async function call<T>(fn: () => Promise<T>): Promise<T> {
    try {
      return await fn()
    } catch (e) {
      throw toApiError(e)
    }
  }

  /** Whether a contact would appear in a page loaded with filter f. Used to
   * decide if a locally-created contact belongs in the currently-shown
   * list; q is an exact-match server-side lookup (see contactWhere on the
   * Go side) that can't be replicated here, so an active search always
   * defers to the next reload instead of guessing. */
  function matchesFilter(c: Contact, f: Filter): boolean {
    if (f.q) return false
    if (f.status && c.status !== f.status) return false
    if (f.source && c.source !== f.source) return false
    return true
  }

  function query(f: Filter): string {
    const q = new URLSearchParams()
    if (f.status) q.set('status', f.status)
    if (f.source) q.set('source', f.source)
    if (f.q) q.set('q', f.q)
    const s = q.toString()
    return s ? `?${s}` : ''
  }

  /** Loads contacts matching filter and every saved segment. Omitting
   * filter reapplies whatever was last requested (see lastFilter). */
  async function load(filter: Filter = lastFilter): Promise<void> {
    lastFilter = filter
    loading.value = true
    error.value = null
    try {
      const [page, segs] = await Promise.all([
        apiFetch<{ contacts: Contact[], counts: Counts }>(`/api/v1/audience/contacts${query(filter)}`),
        apiFetch<Segment[]>('/api/v1/audience/segments'),
      ])
      contacts.value = page.contacts
      counts.value = page.counts
      segments.value = segs ?? []
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  async function createContact(input: ContactInput): Promise<Contact> {
    const c = await call(() => apiFetch<Contact>('/api/v1/audience/contacts', { method: 'POST', body: input }))
    // Only show it in the current list if it actually matches the active
    // filter — otherwise (e.g. adding while viewing the UNSUBSCRIBED tab,
    // where a new contact always defaults to active) it would appear with
    // the wrong badge among contacts that shouldn't be there.
    if (matchesFilter(c, lastFilter)) {
      contacts.value = [c, ...contacts.value]
    }
    // Counts are tenant-wide, not narrowed by the filter (mirrors the Go
    // side's ListContacts convention), so they're bumped either way.
    // Explicit per-status bump, not a dynamic key: c.status is a narrow
    // union, but indexed access on Counts still resolves to
    // `number | undefined` — mirrors the Go Counts.add() switch.
    counts.value.all += 1
    switch (c.status) {
      case 'active': counts.value.active += 1; break
      case 'unsubscribed': counts.value.unsubscribed += 1; break
      case 'bounced': counts.value.bounced += 1; break
      case 'complained': counts.value.complained += 1; break
    }
    return c
  }

  async function updateContact(id: string, input: ContactInput): Promise<void> {
    const c = await call(() => apiFetch<Contact>(`/api/v1/audience/contacts/${id}`, { method: 'PUT', body: input }))
    const i = contacts.value.findIndex(x => x.id === id)
    if (i >= 0) contacts.value[i] = { ...contacts.value[i], ...c }
  }

  async function setStatus(id: string, input: AudienceStatusInput): Promise<void> {
    await call(() => apiFetch(`/api/v1/audience/contacts/${id}/status`, { method: 'POST', body: input }))
    await load()
  }

  async function deleteContact(id: string): Promise<void> {
    await call(() => apiFetch(`/api/v1/audience/contacts/${id}`, { method: 'DELETE' }))
    const removed = contacts.value.find(c => c.id === id)
    contacts.value = contacts.value.filter(c => c.id !== id)
    if (!removed) return
    counts.value.all -= 1
    switch (removed.status) {
      case 'active': counts.value.active -= 1; break
      case 'unsubscribed': counts.value.unsubscribed -= 1; break
      case 'bounced': counts.value.bounced -= 1; break
      case 'complained': counts.value.complained -= 1; break
    }
  }

  async function importCSV(rows: ImportRow[], consent: ContactInput['consent']): Promise<ImportResult> {
    const r = await call(() => apiFetch<ImportResult>('/api/v1/audience/contacts/import', { method: 'POST', body: { rows, consent } }))
    await load()
    return r
  }

  async function createSegment(input: SegmentInput): Promise<Segment> {
    const s = await call(() => apiFetch<Segment>('/api/v1/audience/segments', { method: 'POST', body: input }))
    segments.value = [...segments.value, s]
    return s
  }

  async function updateSegment(id: string, input: SegmentInput): Promise<void> {
    const s = await call(() => apiFetch<Segment>(`/api/v1/audience/segments/${id}`, { method: 'PUT', body: input }))
    const i = segments.value.findIndex(x => x.id === id)
    if (i >= 0) segments.value[i] = s
  }

  async function deleteSegment(id: string): Promise<void> {
    await call(() => apiFetch(`/api/v1/audience/segments/${id}`, { method: 'DELETE' }))
    segments.value = segments.value.filter(s => s.id !== id)
  }

  function exportUrl(filter: Filter = {}): string {
    return `/api/v1/audience/contacts/export.csv${query(filter)}`
  }

  return {
    contacts, counts, segments, loading, error,
    load, createContact, updateContact, setStatus, deleteContact, importCSV,
    createSegment, updateSegment, deleteSegment, exportUrl,
  }
})
