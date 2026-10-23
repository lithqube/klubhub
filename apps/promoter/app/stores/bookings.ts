import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  OpenSlot, OpenSlotInput, OutreachRequest, OutreachTemplate, SendOfferInput,
  Suppression, SuppressionCreate, TalentContact, TalentContactInput, TalentMatch, TemplateCreate, TemplateUpdate,
} from '~/types/bookings'
import type { ApiError } from '~/types/event'
import { toApiError } from '~/utils/api'
import * as api from '~/composables/useBookings'

interface Section { loading: boolean, error: ApiError | null }

const idle = (): Section => ({ loading: false, error: null })

/**
 * The Bookings page store (04.10-05): talent contacts, open slots, slot
 * matches, offer templates, the sent log and the do-not-contact list.
 * talent.* and booking.* are owner/admin/booker actions — the API enforces
 * the same split through OPA.
 */
export const useBookingsStore = defineStore('bookings', () => {
  const talent = ref<TalentContact[]>([])
  const slots = ref<OpenSlot[]>([])
  const matches = ref<TalentMatch[]>([])
  const templates = ref<OutreachTemplate[]>([])
  const offers = ref<OutreachRequest[]>([])
  const suppressions = ref<Suppression[]>([])

  const talentSec = ref<Section>(idle())
  const slotsSec = ref<Section>(idle())
  const matchesSec = ref<Section>(idle())
  const templatesSec = ref<Section>(idle())
  const offersSec = ref<Section>(idle())
  const suppressionsSec = ref<Section>(idle())

  async function call<T>(sec: typeof talentSec, fn: () => Promise<T>): Promise<T> {
    sec.value.loading = true
    sec.value.error = null
    try {
      return await fn()
    } catch (e) {
      sec.value.error = toApiError(e)
      throw sec.value.error
    } finally {
      sec.value.loading = false
    }
  }

  // ---------------------------------------------------------------- talent --

  async function loadTalent(filter: { city?: string, genre?: string, q?: string } = {}): Promise<void> {
    talent.value = await call(talentSec, () => api.listTalent(filter))
  }

  async function createTalent(input: TalentContactInput): Promise<TalentContact> {
    const c = await call(talentSec, () => api.createTalent(input))
    talent.value = [c, ...talent.value]
    return c
  }

  async function updateTalent(id: string, input: TalentContactInput): Promise<TalentContact> {
    const c = await call(talentSec, () => api.updateTalent(id, input))
    const i = talent.value.findIndex(x => x.id === id)
    if (i >= 0) talent.value[i] = c
    return c
  }

  async function deleteTalent(id: string): Promise<void> {
    await call(talentSec, () => api.deleteTalent(id))
    talent.value = talent.value.filter(c => c.id !== id)
  }

  /** One-click opt-out (SPEC §9 v2.1): the contact's email joins the
   * do-not-contact list and the contact leaves the matchable pool. */
  async function suppressTalent(id: string): Promise<Suppression> {
    const s = await call(talentSec, () => api.suppressTalent(id))
    suppressions.value = [s, ...suppressions.value]
    talent.value = talent.value.filter(c => c.id !== id)
    matches.value = matches.value.filter(m => m.contact.id !== id)
    return s
  }

  // ----------------------------------------------------------------- slots --

  async function loadSlots(eventId: string): Promise<void> {
    slots.value = await call(slotsSec, () => api.listSlots(eventId))
  }

  async function createSlot(eventId: string, input: OpenSlotInput): Promise<OpenSlot> {
    const s = await call(slotsSec, () => api.createSlot(eventId, input))
    slots.value = [...slots.value, s]
    return s
  }

  async function deleteSlot(id: string): Promise<void> {
    await call(slotsSec, () => api.deleteSlot(id))
    slots.value = slots.value.filter(s => s.id !== id)
    if (selectedSlotId.value === id) selectedSlotId.value = null
  }

  // --------------------------------------------------------------- matches --

  const selectedSlotId = ref<string | null>(null)

  async function loadMatches(slotId: string): Promise<void> {
    selectedSlotId.value = slotId
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

  async function createTemplate(input: TemplateCreate): Promise<OutreachTemplate> {
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

  async function deleteTemplate(id: string): Promise<void> {
    await call(templatesSec, () => api.deleteTemplate(id))
    templates.value = templates.value.filter(t => t.id !== id)
  }

  // ---------------------------------------------------------------- offers --

  async function loadOffers(): Promise<void> {
    offers.value = await call(offersSec, () => api.listOffers())
  }

  /** Sends one offer and prepends it to the sent log (the log is newest
   * first, like the API's ordering). */
  async function sendOffer(input: SendOfferInput): Promise<OutreachRequest> {
    const o = await call(offersSec, () => api.sendOffer(input))
    offers.value = [o, ...offers.value]
    return o
  }

  // ----------------------------------------------------------- suppressions --

  async function loadSuppressions(): Promise<void> {
    suppressions.value = await call(suppressionsSec, () => api.listSuppressions())
  }

  async function addSuppression(input: SuppressionCreate): Promise<Suppression> {
    const s = await call(suppressionsSec, () => api.addSuppression(input))
    suppressions.value = [s, ...suppressions.value]
    return s
  }

  async function deleteSuppression(id: string): Promise<void> {
    await call(suppressionsSec, () => api.deleteSuppression(id))
    suppressions.value = suppressions.value.filter(s => s.id !== id)
  }

  return {
    talent, slots, matches, templates, offers, suppressions,
    talentSec, slotsSec, matchesSec, templatesSec, offersSec, suppressionsSec,
    selectedSlotId,
    loadTalent, createTalent, updateTalent, deleteTalent, suppressTalent,
    loadSlots, createSlot, deleteSlot,
    loadMatches, clearMatches,
    loadTemplates, createTemplate, updateTemplate, deleteTemplate,
    loadOffers, sendOffer,
    loadSuppressions, addSuppression, deleteSuppression,
  }
})
