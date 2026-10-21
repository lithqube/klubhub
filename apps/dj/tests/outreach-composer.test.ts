// Component tests for the Outreach page (04.10-04): the composer (template
// picker pre-fill, live preview, EPK picker, send + the 503/409/429/422
// error surfaces), the matches list (no-email contacts get no Compose
// button, compose emits contact+slot), the availability panel (render +
// add-form validation) and the sent log (one-click Do not contact posts to
// suppressions). Real Pinia stores, stubbed $fetch.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import OutreachAvailabilityPanel from '../app/components/outreach/OutreachAvailabilityPanel.vue'
import OutreachComposer from '../app/components/outreach/OutreachComposer.vue'
import OutreachMatchesList from '../app/components/outreach/OutreachMatchesList.vue'
import OutreachSentLog from '../app/components/outreach/OutreachSentLog.vue'
import { useOutreachStore } from '../app/stores/outreach'
import { useSettingsStore } from '../app/stores/settings'
import type {
  AvailabilitySlot,
  EpkExport,
  MatchedContact,
  OutreachRequest,
  OutreachTemplate,
  SlotMatch,
  Suppression,
} from '../app/types/outreach'

const fetchMock = vi.fn()
const httpError = (statusCode: number, body: unknown) =>
  Object.assign(new Error(`HTTP ${statusCode}`), { statusCode, data: body })

const slot = (over: Partial<AvailabilitySlot> = {}): AvailabilitySlot => ({
  id: 'as-1', date: '2027-01-16', city: 'Berlin', country: 'DE', genres: ['techno', 'house'],
  fee_min: 600, fee_max: 900, currency: 'EUR', notes: 'Saturday, main room.', created_at: '2026-10-01T10:00:00Z', ...over,
})
const contact = (over: Partial<MatchedContact> = {}): MatchedContact => ({
  id: 'pc-1', name: 'Anna Keller', company: 'Klubnacht Berlin', email: 'anna@klubnacht.example',
  has_email: true, city: 'Berlin', past_gig_in_city: true, ...over,
})
const tpl = (over: Partial<OutreachTemplate> = {}): OutreachTemplate => ({
  id: 'tpl-1', name: 'Default booking request', direction: 'dj_to_promoter',
  subject: 'Booking request: {{dj_name}} in {{city}} on {{date}}',
  body: 'Hi {{promoter_name}},\n\nI\'m {{dj_name}} — free in {{city}} on {{date}}. Fee: {{fee}}.\n\n{{dj_name}} · {{dj_email}}',
  is_default: true, version: 1, created_at: '2026-10-01T10:00:00Z', updated_at: '2026-10-01T10:00:00Z', ...over,
})
const req = (over: Partial<OutreachRequest> = {}): OutreachRequest => ({
  id: 'or-1', contact_id: 'pc-1', availability_slot_id: 'as-1', template_id: 'tpl-1', epk_export_id: 'epk-1',
  email_message_id: 'em-1', to_email: 'anna@klubnacht.example', subject: 'Booking request: SAMPLE_RATE in Berlin on 2027-01-16',
  body: 'Hi Anna Keller,', status: 'sent', sent_at: '2026-10-02T10:00:00Z', created_at: '2026-10-02T10:00:00Z',
  email_status: 'delivered', ...over,
})
const sup = (over: Partial<Suppression> = {}): Suppression => ({
  id: 'sup-1', email: 'stopme@example.com', reason: 'replied "stop"', created_at: '2026-10-01T10:00:00Z', ...over,
})
const epk = (over: Partial<EpkExport> = {}): EpkExport => ({
  id: 'epk-1', garageObjectKey: 'epk/exports/epk-1.pdf', createdAt: '2026-10-01T10:00:00Z', ...over,
})
const match = (over: Partial<SlotMatch> = {}): SlotMatch => ({
  slot: slot(), contacts: [contact()], venues: [{ id: 'v-1', name: 'Klubnacht', city: 'Berlin' }], ...over,
})

const stubs = { KhEmptyState: true }

beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
  const settings = useSettingsStore()
  settings.djName = 'SAMPLE_RATE'
  settings.contactInfo = JSON.stringify({ name: 'SAMPLE_RATE', email: 'samplerate@dj.example', phone: '+49 30 555 0100' })
})

/** Mounts the composer with a contact selected (the page's COMPOSE flow). */
async function mountComposer(props: { contact: MatchedContact | null, availSlot: AvailabilitySlot | null }) {
  const store = useOutreachStore()
  store.templates = [tpl(), tpl({ id: 'tpl-2', name: 'Weekend ask', subject: 'Weekend: {{city}} {{date}}', body: 'Short {{promoter_name}}', is_default: false, version: 2 })]
  store.epkExports = [epk(), epk({ id: 'epk-2', garageObjectKey: 'epk/exports/epk-2.pdf', createdAt: '2026-10-03T10:00:00Z' })]
  const wrapper = mount(OutreachComposer, { props: { contact: null, availSlot: null }, global: { stubs } })
  await wrapper.setProps(props)
  await flushPromises()
  return { wrapper, store }
}

describe('OutreachAvailabilityPanel', () => {
  it('renders the slots and validates the add form before posting', async () => {
    const store = useOutreachStore()
    store.slots = [slot()]
    fetchMock.mockResolvedValue(slot({ id: 'as-2', city: 'Leipzig' }))
    const wrapper = mount(OutreachAvailabilityPanel, { global: { stubs } })
    expect(wrapper.findAll('[data-testid="slot-row"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Berlin')
    expect(wrapper.text()).toContain('600–900 EUR')

    await wrapper.find('[data-testid="slot-add-open"]').trigger('click')
    // jsdom does not submit forms on button clicks — submit the form itself.
    await wrapper.find('[data-testid="slot-form"]').trigger('submit')
    expect(wrapper.find('[data-testid="slot-error"]').text()).toBe('Pick a date.')
    expect(fetchMock).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="slot-date"]').setValue('2027-03-06')
    await wrapper.find('[data-testid="slot-city"]').setValue('Leipzig')
    await wrapper.find('[data-testid="slot-genres"]').setValue('techno, house')
    await wrapper.find('[data-testid="slot-form"]').trigger('submit')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/outreach/availability/slots', expect.objectContaining({
      method: 'POST',
      body: expect.objectContaining({ date: '2027-03-06', city: 'Leipzig', genres: ['techno', 'house'] }),
    }))
    expect(store.slots.map(s => s.id)).toEqual(['as-1', 'as-2'])
  })

  it('deletes a slot after confirm', async () => {
    const store = useOutreachStore()
    store.slots = [slot()]
    fetchMock.mockResolvedValue(null)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(OutreachAvailabilityPanel, { global: { stubs } })
    await wrapper.find('[data-testid="slot-delete"]').trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/outreach/availability/slots/as-1', expect.objectContaining({ method: 'DELETE' }))
    expect(store.slots).toHaveLength(0)
    confirmSpy.mockRestore()
  })
})

describe('OutreachMatchesList', () => {
  it('renders contacts + venue chips; a no-email contact gets a badge and NO Compose button', async () => {
    const store = useOutreachStore()
    store.matches = [match({ contacts: [contact(), contact({ id: 'pc-2', name: 'Dirk Stein', email: '', has_email: false, past_gig_in_city: false })] })]
    const wrapper = mount(OutreachMatchesList, { global: { stubs } })
    const rows = wrapper.findAll('[data-testid="match-row"]')
    expect(rows).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="compose-btn"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="no-email-badge"]').text()).toContain('NO EMAIL')
    expect(wrapper.findAll('[data-testid="venue-chip"]').map(v => v.text())).toEqual(['Klubnacht'])
  })

  it('compose emits the contact and its slot', async () => {
    const store = useOutreachStore()
    store.matches = [match()]
    const wrapper = mount(OutreachMatchesList, { global: { stubs } })
    await wrapper.find('[data-testid="compose-btn"]').trigger('click')
    expect(wrapper.emitted('compose')).toEqual([[{ contact: contact(), slot: slot() }]])
  })
})

describe('OutreachComposer', () => {
  it('shows the empty state without a contact', () => {
    const wrapper = mount(OutreachComposer, { props: { contact: null, availSlot: null }, global: { stubs } })
    expect(wrapper.find('[data-testid="composer-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="composer"]').exists()).toBe(false)
  })

  it('pre-fills a per-request copy from the default template and renders the preview', async () => {
    const { wrapper } = await mountComposer({ contact: contact(), availSlot: slot() })
    expect(wrapper.find('[data-testid="composer-contact"]').text()).toContain('Anna Keller')
    const subject = wrapper.find<HTMLInputElement>('[data-testid="composer-subject"]')
    const body = wrapper.find<HTMLTextAreaElement>('[data-testid="composer-body"]')
    // The editor holds the RAW template copy (variables intact)…
    expect(subject.element.value).toBe('Booking request: {{dj_name}} in {{city}} on {{date}}')
    expect(body.element.value).toContain('{{promoter_name}}')
    // …while the preview renders the variables client-side.
    const preview = wrapper.find('[data-testid="composer-preview"]').text()
    expect(preview).toContain('Booking request: SAMPLE_RATE in Berlin on 2027-01-16')
    expect(preview).toContain('Hi Anna Keller,')
    expect(preview).toContain('samplerate@dj.example')
    expect(preview).toContain('600–900 EUR')
    expect(preview).not.toContain('{{dj_name}}')

    // Editing the copy re-renders the preview live.
    await body.setValue('Hi {{promoter_name}}, still free on {{date}}?')
    expect(wrapper.find('[data-testid="composer-preview"]').text()).toContain('Hi Anna Keller, still free on 2027-01-16?')
  })

  it('switching the template picker re-fills the copy; the EPK picker lists exports', async () => {
    const { wrapper } = await mountComposer({ contact: contact(), availSlot: slot() })
    await wrapper.find('[data-testid="composer-template"]').setValue('tpl-2')
    expect(wrapper.find<HTMLInputElement>('[data-testid="composer-subject"]').element.value).toBe('Weekend: {{city}} {{date}}')
    const options = wrapper.findAll('[data-testid="composer-epk"] option')
    expect(options).toHaveLength(3) // — no attachment — + 2 exports
    await wrapper.find('[data-testid="composer-epk"]').setValue('epk-2')
    expect(wrapper.find('[data-testid="composer-epk-badge"]').text()).toContain('epk-2.pdf')
  })

  it('sends the request with the right body, shows the notice and prepends the log', async () => {
    const { wrapper, store } = await mountComposer({ contact: contact(), availSlot: slot() })
    fetchMock.mockResolvedValue(req({ id: 'or-new' }))
    await wrapper.find('[data-testid="composer-epk"]').setValue('epk-1')
    await wrapper.find('[data-testid="composer-send"]').trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/outreach/requests', expect.objectContaining({
      method: 'POST',
      body: {
        contact_id: 'pc-1',
        availability_slot_id: 'as-1',
        template_id: 'tpl-1',
        epk_export_id: 'epk-1',
        subject: 'Booking request: {{dj_name}} in {{city}} on {{date}}',
        body: expect.stringContaining('{{promoter_name}}'),
      },
    }))
    expect(wrapper.find('[data-testid="composer-notice"]').text()).toBe('Request sent to Anna Keller.')
    expect(store.requests[0]?.id).toBe('or-new')
  })

  it.each([
    [503, { error: 'email is not available' }, 'Email is not configured (Plunk)'],
    [409, { error: 'address is suppressed' }, 'This contact is on your do-not-contact list.'],
    [409, { error: 'conflict' }, 'The template changed since you loaded it'],
    [429, { error: 'daily outreach limit reached' }, 'Daily outreach limit reached — try again tomorrow.'],
    [422, { error: 'contact has no email address' }, 'This contact has no email address on file.'],
    [422, { error: 'no DJ contact email configured' }, 'Set your contact email in Settings'],
    [422, { error: 'epk export has no garage object key' }, 'This EPK export has no file'],
    [404, { error: 'not found' }, 'That contact or slot no longer exists'],
  ])('surfaces the %i error (%s)', async (status, body, expected) => {
    const { wrapper } = await mountComposer({ contact: contact(), availSlot: slot() })
    fetchMock.mockRejectedValue(httpError(status, body))
    await wrapper.find('[data-testid="composer-send"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="composer-error"]').text()).toContain(expected)
    expect(wrapper.find('[data-testid="composer-notice"]').exists()).toBe(false)
  })
})

describe('OutreachSentLog', () => {
  it('renders rows with delivery status and EPK badge', () => {
    const store = useOutreachStore()
    store.requests = [req(), req({ id: 'or-2', epk_export_id: null, status: 'failed', email_status: 'failed', email_error: 'mailbox full', sent_at: null })]
    const wrapper = mount(OutreachSentLog, { global: { stubs } })
    const rows = wrapper.findAll('[data-testid="request-row"]')
    expect(rows).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="request-status"]').map(s => s.text())).toEqual(['DELIVERED', 'FAILED'])
    expect(wrapper.findAll('[data-testid="request-epk"]')).toHaveLength(1)
    expect(rows[1]!.text()).toContain('mailbox full')
  })

  it('one-click DO NOT CONTACT posts the recipient email to suppressions (after confirm)', async () => {
    const store = useOutreachStore()
    store.requests = [req()]
    fetchMock.mockResolvedValue(sup({ id: 'sup-new', email: 'anna@klubnacht.example' }))
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(OutreachSentLog, { global: { stubs } })
    await wrapper.find('[data-testid="request-suppress"]').trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/outreach/suppressions', expect.objectContaining({
      method: 'POST',
      body: { email: 'anna@klubnacht.example', reason: 'opted out via the sent log' },
    }))
    expect(store.suppressions[0]?.email).toBe('anna@klubnacht.example')
    confirmSpy.mockRestore()
  })

  it('does nothing when the confirm is declined', async () => {
    const store = useOutreachStore()
    store.requests = [req()]
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mount(OutreachSentLog, { global: { stubs } })
    await wrapper.find('[data-testid="request-suppress"]').trigger('click')
    await flushPromises()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(store.suppressions).toHaveLength(0)
    confirmSpy.mockRestore()
  })
})
