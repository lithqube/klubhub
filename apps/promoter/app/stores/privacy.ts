import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ApiError } from '~/types/event'
import type { EventPrivacy, PurgeResponse, RetentionOverview, RetentionPreview } from '~/types/privacy'
import { apiFetch, toApiError } from '~/utils/api'

/**
 * Privacy and retention (P2.5): the org retention period with upcoming and
 * recent purges (settings), each event's purge state (guests, report and
 * door tabs) and ERASE NOW.
 */
export const usePrivacyStore = defineStore('privacy', () => {
  const retention = ref<RetentionOverview | null>(null)
  const retentionLoading = ref(false)
  const retentionError = ref<ApiError | null>(null)

  /** Purge state per event id. */
  const events = ref<Record<string, EventPrivacy>>({})
  const eventErrors = ref<Record<string, ApiError>>({})

  async function fetchRetention(): Promise<void> {
    retentionLoading.value = true
    retentionError.value = null
    try {
      retention.value = await apiFetch<RetentionOverview>('/api/v1/org/retention')
    } catch (e) {
      retentionError.value = toApiError(e)
    } finally {
      retentionLoading.value = false
    }
  }

  /**
   * Which ended events saving `days` would erase at once (only a shorter
   * period can). Throws an ApiError.
   */
  async function previewRetention(days: number): Promise<RetentionPreview> {
    try {
      const p = await apiFetch<RetentionPreview>('/api/v1/org/retention/preview', { query: { days } })
      return { would_purge: p?.would_purge ?? [], count: p?.count ?? 0 }
    } catch (e) {
      throw toApiError(e)
    }
  }

  /**
   * Save the retention period (1–365 days); throws an ApiError. A shorter
   * period that erases ended events at once needs `confirmPurge` = their
   * count (else 409 retention_would_purge) and a recent sign-in. The server
   * recomputes the dates of events not yet erased, so the overview is
   * reloaded (a failed reload keeps the saved value).
   */
  async function saveRetention(days: number, confirmPurge?: number): Promise<void> {
    const body: { retention_days: number, confirm_purge?: number } = { retention_days: days }
    if (confirmPurge) body.confirm_purge = confirmPurge
    try {
      await apiFetch('/api/v1/org/retention', { method: 'PUT', body })
    } catch (e) {
      throw toApiError(e)
    }
    if (retention.value) retention.value = { ...retention.value, retention_days: days }
    // Cached event dates are stale now.
    events.value = {}
    await fetchRetention()
    if (retentionError.value && retention.value) retentionError.value = null
  }

  /** Load one event's purge state; never throws (the error is kept per event). */
  async function fetchEvent(id: string): Promise<EventPrivacy | null> {
    const { [id]: _drop, ...rest } = eventErrors.value
    eventErrors.value = rest
    try {
      const p = await apiFetch<EventPrivacy>(`/api/v1/events/${id}/privacy`)
      events.value = { ...events.value, [id]: p }
      return p
    } catch (e) {
      eventErrors.value = { ...eventErrors.value, [id]: toApiError(e) }
      return null
    }
  }

  /**
   * Erase an ended event's guest names and contacts now. `confirm` must be
   * the event title; the server also wants a recent sign-in (step-up).
   * Throws an ApiError; on success the event's purge state is reloaded.
   */
  async function purgeEvent(id: string, confirm: string): Promise<PurgeResponse> {
    let r: PurgeResponse | null
    try {
      r = await apiFetch<PurgeResponse | null>(`/api/v1/events/${id}/purge`, { method: 'POST', body: { confirm } })
    } catch (e) {
      throw toApiError(e)
    }
    const p = await fetchEvent(id)
    const purgedAt = r?.purged_at ?? p?.purged_at ?? new Date().toISOString()
    if (!p?.purged_at) {
      const prev = events.value[id]
      events.value = { ...events.value, [id]: { purge_after: prev?.purge_after ?? null, retention_days: prev?.retention_days ?? 30, personal_rows: 0, purged_at: purgedAt } }
    }
    return { purged_at: purgedAt, counts: r?.counts }
  }

  return { retention, retentionLoading, retentionError, events, eventErrors, fetchRetention, previewRetention, saveRetention, fetchEvent, purgeEvent }
})
