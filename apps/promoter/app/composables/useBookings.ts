// Typed wrappers for the bookings API (04.10-03 routes at
// /api/v1/bookings/*). All access goes through apiFetch, so the CSRF header
// and the SSR cookie forwarding apply. Errors surface as the raw $fetch
// error; callers normalise with toApiError.
import type {
  OpenSlot, OpenSlotInput, OutreachRequest, OutreachTemplate, SendOfferInput,
  Suppression, SuppressionCreate, TalentContact, TalentContactInput, TalentMatch, TemplateCreate, TemplateUpdate,
} from '~/types/bookings'
import { apiFetch } from '~/utils/api'

export function listTalent(filter: { city?: string, genre?: string, q?: string } = {}): Promise<TalentContact[]> {
  return apiFetch<TalentContact[]>('/api/v1/bookings/talent', { query: filter })
}

export function createTalent(input: TalentContactInput): Promise<TalentContact> {
  return apiFetch<TalentContact>('/api/v1/bookings/talent', { method: 'POST', body: input })
}

export function updateTalent(id: string, input: TalentContactInput): Promise<TalentContact> {
  return apiFetch<TalentContact>(`/api/v1/bookings/talent/${id}`, { method: 'PUT', body: input })
}

export async function deleteTalent(id: string): Promise<void> {
  await apiFetch<null>(`/api/v1/bookings/talent/${id}`, { method: 'DELETE' })
}

/** One-click opt-out: adds the contact's email to the do-not-contact list. */
export function suppressTalent(id: string): Promise<Suppression> {
  return apiFetch<Suppression>(`/api/v1/bookings/talent/${id}/suppress`, { method: 'POST' })
}

export function listSlots(eventId: string): Promise<OpenSlot[]> {
  return apiFetch<OpenSlot[]>(`/api/v1/bookings/events/${eventId}/slots`)
}

export function createSlot(eventId: string, input: OpenSlotInput): Promise<OpenSlot> {
  return apiFetch<OpenSlot>(`/api/v1/bookings/events/${eventId}/slots`, { method: 'POST', body: input })
}

export async function deleteSlot(id: string): Promise<void> {
  await apiFetch<null>(`/api/v1/bookings/slots/${id}`, { method: 'DELETE' })
}

export function fetchMatches(slotId: string): Promise<TalentMatch[]> {
  return apiFetch<TalentMatch[]>(`/api/v1/bookings/slots/${slotId}/matches`)
}

export function listTemplates(): Promise<OutreachTemplate[]> {
  return apiFetch<OutreachTemplate[]>('/api/v1/bookings/templates')
}

export function createTemplate(input: TemplateCreate): Promise<OutreachTemplate> {
  return apiFetch<OutreachTemplate>('/api/v1/bookings/templates', { method: 'POST', body: input })
}

export function updateTemplate(id: string, input: TemplateUpdate): Promise<OutreachTemplate> {
  return apiFetch<OutreachTemplate>(`/api/v1/bookings/templates/${id}`, { method: 'PUT', body: input })
}

export async function deleteTemplate(id: string): Promise<void> {
  await apiFetch<null>(`/api/v1/bookings/templates/${id}`, { method: 'DELETE' })
}

export function sendOffer(input: SendOfferInput): Promise<OutreachRequest> {
  return apiFetch<OutreachRequest>('/api/v1/bookings/offers', { method: 'POST', body: input })
}

export function listOffers(): Promise<OutreachRequest[]> {
  return apiFetch<OutreachRequest[]>('/api/v1/bookings/offers')
}

export function getOffer(id: string): Promise<OutreachRequest> {
  return apiFetch<OutreachRequest>(`/api/v1/bookings/offers/${id}`)
}

export function listSuppressions(): Promise<Suppression[]> {
  return apiFetch<Suppression[]>('/api/v1/bookings/suppressions')
}

export function addSuppression(input: SuppressionCreate): Promise<Suppression> {
  return apiFetch<Suppression>('/api/v1/bookings/suppressions', { method: 'POST', body: input })
}

export async function deleteSuppression(id: string): Promise<void> {
  await apiFetch<null>(`/api/v1/bookings/suppressions/${id}`, { method: 'DELETE' })
}
