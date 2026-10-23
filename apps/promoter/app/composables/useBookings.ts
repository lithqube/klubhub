// Typed wrappers for the bookings API (04.10-03 routes at
// /api/v1/bookings/*). All access goes through apiFetch, so the CSRF header
// and the SSR cookie forwarding apply. Errors surface as the raw $fetch
// error; callers normalise with toApiError.
import type {
  OpenSlot, OpenSlotInput, OutreachRequest, OutreachTemplate, SendOfferInput,
  Suppression, SuppressionCreate, TalentContact, TalentContactInput, TalentMatch, TemplateCreate, TemplateUpdate,
} from '~/types/bookings'
import { apiFetch } from '~/utils/api'

// A dead backend makes the Nuxt proxy hang rather than refuse, which would
// leave a store section "loading" forever. Time requests out so the store
// surfaces its error state (with RETRY). Sends also deliver the email, so
// they get a longer budget.
const REQUEST_TIMEOUT_MS = 15_000
const SEND_TIMEOUT_MS = 60_000

type FetchOptions = NonNullable<Parameters<typeof apiFetch>[1]>

function call<T>(url: string, opts: FetchOptions = {}): Promise<T> {
  return apiFetch<T>(url, { timeout: REQUEST_TIMEOUT_MS, ...opts })
}

export function listTalent(filter: { city?: string, genre?: string, q?: string } = {}): Promise<TalentContact[]> {
  return call<TalentContact[]>('/api/v1/bookings/talent', { query: filter })
}

export function createTalent(input: TalentContactInput): Promise<TalentContact> {
  return call<TalentContact>('/api/v1/bookings/talent', { method: 'POST', body: input })
}

export function updateTalent(id: string, input: TalentContactInput): Promise<TalentContact> {
  return call<TalentContact>(`/api/v1/bookings/talent/${id}`, { method: 'PUT', body: input })
}

export async function deleteTalent(id: string): Promise<void> {
  await call<null>(`/api/v1/bookings/talent/${id}`, { method: 'DELETE' })
}

/** One-click opt-out: adds the contact's email to the do-not-contact list. */
export function suppressTalent(id: string): Promise<Suppression> {
  return call<Suppression>(`/api/v1/bookings/talent/${id}/suppress`, { method: 'POST' })
}

export function listSlots(eventId: string): Promise<OpenSlot[]> {
  return call<OpenSlot[]>(`/api/v1/bookings/events/${eventId}/slots`)
}

export function createSlot(eventId: string, input: OpenSlotInput): Promise<OpenSlot> {
  return call<OpenSlot>(`/api/v1/bookings/events/${eventId}/slots`, { method: 'POST', body: input })
}

export async function deleteSlot(id: string): Promise<void> {
  await call<null>(`/api/v1/bookings/slots/${id}`, { method: 'DELETE' })
}

export function fetchMatches(slotId: string): Promise<TalentMatch[]> {
  return call<TalentMatch[]>(`/api/v1/bookings/slots/${slotId}/matches`)
}

export function listTemplates(): Promise<OutreachTemplate[]> {
  return call<OutreachTemplate[]>('/api/v1/bookings/templates')
}

export function createTemplate(input: TemplateCreate): Promise<OutreachTemplate> {
  return call<OutreachTemplate>('/api/v1/bookings/templates', { method: 'POST', body: input })
}

export function updateTemplate(id: string, input: TemplateUpdate): Promise<OutreachTemplate> {
  return call<OutreachTemplate>(`/api/v1/bookings/templates/${id}`, { method: 'PUT', body: input })
}

export async function deleteTemplate(id: string): Promise<void> {
  await call<null>(`/api/v1/bookings/templates/${id}`, { method: 'DELETE' })
}

export function sendOffer(input: SendOfferInput): Promise<OutreachRequest> {
  return call<OutreachRequest>('/api/v1/bookings/offers', { method: 'POST', body: input, timeout: SEND_TIMEOUT_MS })
}

export function listOffers(): Promise<OutreachRequest[]> {
  return call<OutreachRequest[]>('/api/v1/bookings/offers')
}

export function getOffer(id: string): Promise<OutreachRequest> {
  return call<OutreachRequest>(`/api/v1/bookings/offers/${id}`)
}

export function listSuppressions(): Promise<Suppression[]> {
  return call<Suppression[]>('/api/v1/bookings/suppressions')
}

export function addSuppression(input: SuppressionCreate): Promise<Suppression> {
  return call<Suppression>('/api/v1/bookings/suppressions', { method: 'POST', body: input })
}

export async function deleteSuppression(id: string): Promise<void> {
  await call<null>(`/api/v1/bookings/suppressions/${id}`, { method: 'DELETE' })
}
