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

// A dead backend makes the Nuxt proxy hang rather than refuse, which would
// leave a store section "loading" forever. Time requests out so the store
// surfaces its error state (with RETRY). Sends also deliver the email and may
// read the EPK object, so they get a longer budget.
const REQUEST_TIMEOUT_MS = 15_000
const SEND_TIMEOUT_MS = 60_000

type FetchOptions = NonNullable<Parameters<typeof $fetch>[1]>

function call<T>(url: string, opts: FetchOptions = {}): Promise<T> {
  return $fetch<T>(url, { timeout: REQUEST_TIMEOUT_MS, ...opts } as Parameters<typeof $fetch>[1]) as Promise<T>
}

// ─── Availability slots ──────────────────────────────────────────────────────

export function listSlots(): Promise<AvailabilitySlot[]> {
  return call<AvailabilitySlot[]>('/api/v1/outreach/availability/slots')
}

export function createSlot(input: SlotCreate): Promise<AvailabilitySlot> {
  return call<AvailabilitySlot>('/api/v1/outreach/availability/slots', { method: 'POST', body: input })
}

export async function deleteSlot(id: string): Promise<void> {
  await call<null>(`/api/v1/outreach/availability/slots/${id}`, { method: 'DELETE' })
}

/** Matches for one slot (?slot_id=) or every slot. */
export function fetchMatches(slotId?: string): Promise<SlotMatch[]> {
  return call<SlotMatch[]>('/api/v1/outreach/availability/matches', {
    query: slotId ? { slot_id: slotId } : undefined,
  })
}

// ─── Templates ───────────────────────────────────────────────────────────────

export function listTemplates(): Promise<OutreachTemplate[]> {
  return call<OutreachTemplate[]>('/api/v1/outreach/templates')
}

export function createTemplate(input: TemplateCreate): Promise<OutreachTemplate> {
  return call<OutreachTemplate>('/api/v1/outreach/templates', { method: 'POST', body: input })
}

export function updateTemplate(id: string, input: TemplateUpdate): Promise<OutreachTemplate> {
  return call<OutreachTemplate>(`/api/v1/outreach/templates/${id}`, { method: 'PUT', body: input })
}

export async function deleteTemplate(id: string): Promise<void> {
  await call<null>(`/api/v1/outreach/templates/${id}`, { method: 'DELETE' })
}

// ─── Requests (sent log) ─────────────────────────────────────────────────────

export function sendRequest(input: SendRequestInput): Promise<OutreachRequest> {
  return call<OutreachRequest>('/api/v1/outreach/requests', { method: 'POST', body: input, timeout: SEND_TIMEOUT_MS })
}

export function listRequests(): Promise<OutreachRequest[]> {
  return call<OutreachRequest[]>('/api/v1/outreach/requests')
}

export function getRequest(id: string): Promise<OutreachRequest> {
  return call<OutreachRequest>(`/api/v1/outreach/requests/${id}`)
}

// ─── Suppressions (do-not-contact list) ──────────────────────────────────────

export function listSuppressions(): Promise<Suppression[]> {
  return call<Suppression[]>('/api/v1/outreach/suppressions')
}

export function addSuppression(input: SuppressionCreate): Promise<Suppression> {
  return call<Suppression>('/api/v1/outreach/suppressions', { method: 'POST', body: input })
}

export async function deleteSuppression(id: string): Promise<void> {
  await call<null>(`/api/v1/outreach/suppressions/${id}`, { method: 'DELETE' })
}

// ─── EPK exports (the composer's attach picker) ──────────────────────────────

export function listEpkExports(): Promise<EpkExport[]> {
  return call<{ data: EpkExport[] }>('/api/v1/epk/exports').then(r => r.data ?? [])
}
