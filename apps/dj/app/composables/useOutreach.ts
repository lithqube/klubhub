// Typed wrappers for the outreach API (04.10-02 routes mounted at
// /api/v1/outreach). The Nuxt proxy forwards to the Go API in prod; the
// Nitro mocks under server/api/v1/outreach serve the same shapes in dev.
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

// ─── Availability slots ──────────────────────────────────────────────────────

export function listSlots(): Promise<AvailabilitySlot[]> {
  return $fetch<AvailabilitySlot[]>('/api/v1/outreach/availability/slots')
}

export function createSlot(input: SlotCreate): Promise<AvailabilitySlot> {
  return $fetch<AvailabilitySlot>('/api/v1/outreach/availability/slots', { method: 'POST', body: input })
}

export async function deleteSlot(id: string): Promise<void> {
  await $fetch<null>(`/api/v1/outreach/availability/slots/${id}`, { method: 'DELETE' })
}

/** Matches for one slot (?slot_id=) or every slot. */
export function fetchMatches(slotId?: string): Promise<SlotMatch[]> {
  return $fetch<SlotMatch[]>('/api/v1/outreach/availability/matches', {
    query: slotId ? { slot_id: slotId } : undefined,
  })
}

// ─── Templates ───────────────────────────────────────────────────────────────

export function listTemplates(): Promise<OutreachTemplate[]> {
  return $fetch<OutreachTemplate[]>('/api/v1/outreach/templates')
}

export function createTemplate(input: TemplateCreate): Promise<OutreachTemplate> {
  return $fetch<OutreachTemplate>('/api/v1/outreach/templates', { method: 'POST', body: input })
}

export function updateTemplate(id: string, input: TemplateUpdate): Promise<OutreachTemplate> {
  return $fetch<OutreachTemplate>(`/api/v1/outreach/templates/${id}`, { method: 'PUT', body: input })
}

export async function deleteTemplate(id: string): Promise<void> {
  await $fetch<null>(`/api/v1/outreach/templates/${id}`, { method: 'DELETE' })
}

// ─── Requests (sent log) ─────────────────────────────────────────────────────

export function sendRequest(input: SendRequestInput): Promise<OutreachRequest> {
  return $fetch<OutreachRequest>('/api/v1/outreach/requests', { method: 'POST', body: input })
}

export function listRequests(): Promise<OutreachRequest[]> {
  return $fetch<OutreachRequest[]>('/api/v1/outreach/requests')
}

export function getRequest(id: string): Promise<OutreachRequest> {
  return $fetch<OutreachRequest>(`/api/v1/outreach/requests/${id}`)
}

// ─── Suppressions (do-not-contact list) ──────────────────────────────────────

export function listSuppressions(): Promise<Suppression[]> {
  return $fetch<Suppression[]>('/api/v1/outreach/suppressions')
}

export function addSuppression(input: SuppressionCreate): Promise<Suppression> {
  return $fetch<Suppression>('/api/v1/outreach/suppressions', { method: 'POST', body: input })
}

export async function deleteSuppression(id: string): Promise<void> {
  await $fetch<null>(`/api/v1/outreach/suppressions/${id}`, { method: 'DELETE' })
}

// ─── EPK exports (the composer's attach picker) ──────────────────────────────

export function listEpkExports(): Promise<EpkExport[]> {
  return $fetch<{ data: EpkExport[] }>('/api/v1/epk/exports').then(r => r.data ?? [])
}
