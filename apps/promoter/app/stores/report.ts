import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ApiError } from '~/types/event'
import type { EventReport } from '~/types/report'
import { apiFetch, toApiError } from '~/utils/api'

/** The post-event report of one event at a time (P2.4); read-only. */
export const useReportStore = defineStore('report', () => {
  const eventId = ref<string | null>(null)
  const report = ref<EventReport | null>(null)
  const loading = ref(false)
  const error = ref<ApiError | null>(null)
  /** Allocations whose list-back CSV is being prepared (several may run at once). */
  const busy = ref(new Set<string>())

  const base = (id: string) => `/api/v1/events/${id}/report`

  async function load(id: string): Promise<void> {
    // A different event never shows the previous event's numbers.
    if (eventId.value !== id) report.value = null
    eventId.value = id
    loading.value = true
    error.value = null
    try {
      report.value = await apiFetch<EventReport>(base(id))
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  /** CSV text of one allocation's guests (server-hardened, audited). No email or phone. */
  async function listBackCsv(id: string, allocationId: string): Promise<string> {
    busy.value.add(allocationId)
    try {
      return await apiFetch<string>(`${base(id)}/list-back.csv`, { query: { allocation_id: allocationId }, responseType: 'text' })
    } catch (e) {
      throw toApiError(e)
    } finally {
      busy.value.delete(allocationId)
    }
  }

  return { eventId, report, loading, error, busy, load, listBackCsv }
})
