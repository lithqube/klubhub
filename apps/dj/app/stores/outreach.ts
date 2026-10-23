import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  AvailabilitySlot,
  EpkExport,
  OutreachRequest,
  OutreachTemplate,
  SendRequestInput,
  SlotCreate,
  SlotMatch,
  Suppression,
  SuppressionCreate,
  TemplateCreate,
  TemplateUpdate,
} from '~/types/outreach'
import * as api from '~/composables/useOutreach'

interface Section { loading: boolean, error: unknown }

const idle = (): Section => ({ loading: false, error: null })

/**
 * The Outreach page store (04.10-04): availability slots, slot matches,
 * booking-request templates, the sent log, the do-not-contact list and the
 * EPK export picker's options. The DJ side is single-user — no role gate.
 */
export const useOutreachStore = defineStore('outreach', () => {
  const slots = ref<AvailabilitySlot[]>([])
  const matches = ref<SlotMatch[]>([])
  const templates = ref<OutreachTemplate[]>([])
  const requests = ref<OutreachRequest[]>([])
  const suppressions = ref<Suppression[]>([])
  const epkExports = ref<EpkExport[]>([])

  const slotsSec = ref<Section>(idle())
  const matchesSec = ref<Section>(idle())
  const templatesSec = ref<Section>(idle())
  const requestsSec = ref<Section>(idle())
  const suppressionsSec = ref<Section>(idle())
  const epkSec = ref<Section>(idle())

  async function call<T>(sec: typeof slotsSec, fn: () => Promise<T>): Promise<T> {
    sec.value.loading = true
    sec.value.error = null
    try {
      return await fn()
    } catch (e) {
      sec.value.error = e
      throw e
    } finally {
      sec.value.loading = false
    }
  }

  // ----------------------------------------------------------------- slots --

  async function loadSlots(): Promise<void> {
    slots.value = await call(slotsSec, () => api.listSlots())
  }

  async function addSlot(input: SlotCreate): Promise<AvailabilitySlot> {
    const s = await call(slotsSec, () => api.createSlot(input))
    slots.value = [...slots.value, s]
    return s
  }

  async function removeSlot(id: string): Promise<void> {
    await call(slotsSec, () => api.deleteSlot(id))
    slots.value = slots.value.filter(s => s.id !== id)
    matches.value = matches.value.filter(m => m.slot.id !== id)
    if (selectedSlotId.value === id) selectedSlotId.value = null
  }

  // --------------------------------------------------------------- matches --

  const selectedSlotId = ref<string | null>(null)

  /** Matches for one slot, or every slot when no id is given. */
  async function loadMatches(slotId?: string): Promise<void> {
    selectedSlotId.value = slotId ?? null
    matches.value = await call(matchesSec, () => api.fetchMatches(slotId))
  }

  function clearMatches(): void {
    selectedSlotId.value = null
    matches.value = []
  }

  // ------------------------------------------------------------- templates --

  async function loadTemplates(): Promise<void> {
    templates.value = await call(templatesSec, () => api.listTemplates())
  }

  async function saveTemplate(input: TemplateCreate): Promise<OutreachTemplate> {
    const t = await call(templatesSec, () => api.createTemplate(input))
    templates.value = [...templates.value, t]
    return t
  }

  async function updateTemplate(id: string, input: TemplateUpdate): Promise<OutreachTemplate> {
    const t = await call(templatesSec, () => api.updateTemplate(id, input))
    const i = templates.value.findIndex(x => x.id === id)
    if (i >= 0) templates.value[i] = t
    return t
  }

  async function removeTemplate(id: string): Promise<void> {
    await call(templatesSec, () => api.deleteTemplate(id))
    templates.value = templates.value.filter(t => t.id !== id)
  }

  // -------------------------------------------------------------- requests --

  async function loadRequests(): Promise<void> {
    requests.value = await call(requestsSec, () => api.listRequests())
  }

  /** Sends one booking request and prepends it to the sent log (the log is
   * newest first, like the API's ordering). */
  async function sendRequest(input: SendRequestInput): Promise<OutreachRequest> {
    const r = await call(requestsSec, () => api.sendRequest(input))
    requests.value = [r, ...requests.value]
    return r
  }

  // ----------------------------------------------------------- suppressions --

  async function loadSuppressions(): Promise<void> {
    suppressions.value = await call(suppressionsSec, () => api.listSuppressions())
  }

  /** One-click opt-out (SPEC §9 v2.1): the recipient's email joins the
   * do-not-contact list and leaves the matchable pool. */
  async function addSuppression(input: SuppressionCreate): Promise<Suppression> {
    const s = await call(suppressionsSec, () => api.addSuppression(input))
    if (!suppressions.value.some(x => x.email.toLowerCase() === s.email.toLowerCase())) {
      suppressions.value = [s, ...suppressions.value]
    }
    return s
  }

  async function removeSuppression(id: string): Promise<void> {
    await call(suppressionsSec, () => api.deleteSuppression(id))
    suppressions.value = suppressions.value.filter(s => s.id !== id)
  }

  // ------------------------------------------------------------ EPK picker --

  async function loadEpkExports(): Promise<void> {
    epkExports.value = await call(epkSec, () => api.listEpkExports())
  }

  return {
    slots, matches, templates, requests, suppressions, epkExports,
    slotsSec, matchesSec, templatesSec, requestsSec, suppressionsSec, epkSec,
    selectedSlotId,
    loadSlots, addSlot, removeSlot,
    loadMatches, clearMatches,
    loadTemplates, saveTemplate, updateTemplate, removeTemplate,
    loadRequests, sendRequest,
    loadSuppressions, addSuppression, removeSuppression,
    loadEpkExports,
  }
})
