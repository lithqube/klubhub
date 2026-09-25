import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  ApiError, EventDetail, EventInput, EventStatus, EventSummary, EventView, ExportData, LineupInput, StageInput,
} from '~/types/event'
import { apiFetch, toApiError } from '~/utils/api'

export const useEventStore = defineStore('event', () => {
  const lists = ref<Record<EventView, EventSummary[]>>({ upcoming: [], drafts: [], past: [] })
  const loading = ref(false)
  const error = ref<ApiError | null>(null)
  const current = ref<EventDetail | null>(null)

  async function fetchList(view: EventView): Promise<void> {
    loading.value = true
    error.value = null
    try {
      lists.value[view] = await apiFetch<EventSummary[]>('/api/v1/events', { query: { view } })
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  async function fetchEvent(id: string): Promise<EventDetail | null> {
    error.value = null
    try {
      current.value = await apiFetch<EventDetail>(`/api/v1/events/${id}`)
    } catch (e) {
      current.value = null
      error.value = toApiError(e)
    }
    return current.value
  }

  async function run(fn: () => Promise<EventDetail>): Promise<EventDetail> {
    try {
      current.value = await fn()
      return current.value
    } catch (e) {
      throw toApiError(e)
    }
  }

  const createEvent = (input: EventInput) =>
    run(() => apiFetch<EventDetail>('/api/v1/events', { method: 'POST', body: input }))

  const updateEvent = (id: string, version: number, input: EventInput) =>
    run(() => apiFetch<EventDetail>(`/api/v1/events/${id}`, { method: 'PUT', body: { version, event: input } }))

  const changeStatus = (id: string, version: number, status: EventStatus) =>
    run(() => apiFetch<EventDetail>(`/api/v1/events/${id}/status`, { method: 'POST', body: { version, status } }))

  const replaceStages = (id: string, version: number, stages: StageInput[]) =>
    run(() => apiFetch<EventDetail>(`/api/v1/events/${id}/stages`, { method: 'PUT', body: { version, stages } }))

  const replaceLineup = (id: string, version: number, lineup: LineupInput[]) =>
    run(() => apiFetch<EventDetail>(`/api/v1/events/${id}/lineup`, { method: 'PUT', body: { version, lineup } }))

  async function fetchExport(id: string): Promise<ExportData> {
    try {
      return await apiFetch<ExportData>(`/api/v1/events/${id}/export`)
    } catch (e) {
      throw toApiError(e)
    }
  }

  async function fetchJsonLd(id: string): Promise<Record<string, unknown>> {
    try {
      return await apiFetch<Record<string, unknown>>(`/api/v1/events/${id}/export/jsonld`)
    } catch (e) {
      throw toApiError(e)
    }
  }

  async function fetchIcs(id: string): Promise<string> {
    try {
      return await apiFetch<string>(`/api/v1/events/${id}/export/ics`, { responseType: 'text' })
    } catch (e) {
      throw toApiError(e)
    }
  }

  return {
    lists, loading, error, current,
    fetchList, fetchEvent, createEvent, updateEvent, changeStatus, replaceStages, replaceLineup,
    fetchExport, fetchJsonLd, fetchIcs,
  }
})
