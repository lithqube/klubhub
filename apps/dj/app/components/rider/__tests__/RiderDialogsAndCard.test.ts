import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RiderTemplateDialog from '../RiderTemplateDialog.vue'
import AttachRiderDialog from '../AttachRiderDialog.vue'
import RiderAttachmentCard from '../RiderAttachmentCard.vue'
import { useRiderStore } from '../../../stores/rider'
import type { Gig } from '../../../types/gig'
import type { RiderAttachment, RiderTemplate } from '../../../types/rider'

// Real Pinia store, stubbed transport. Covers the wiring bugs found in review:
// a dialog that never rendered, pickers/cards that never loaded their data.

const template = (over: Partial<RiderTemplate> = {}): RiderTemplate => ({
  id: 'tmpl-1', name: 'Standard club', technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T0', ...over,
})
const attachment = (over: Partial<RiderAttachment> = {}): RiderAttachment => ({
  id: 'att-1', gigId: 'gig-1', templateId: 'tmpl-1', technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T0', ...over,
})
const gig = { id: 'gig-1' } as Gig

type Call = [string, { method?: string; body?: unknown }?]
const fetchMock = () => vi.mocked($fetch as unknown as (url: string, o?: Call[1]) => Promise<unknown>)
const urls = () => fetchMock().mock.calls.map(c => `${c[1]?.method ?? 'GET'} ${c[0]}`)

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(() => { vi.unstubAllGlobals() })

describe('RiderTemplateDialog', () => {
  it('renders when open is true (the prop must be declared) and not when false', async () => {
    const closed = mount(RiderTemplateDialog, { props: { open: false } })
    expect(closed.find('[data-testid="rider-template-dialog"]').exists()).toBe(false)
    const open = mount(RiderTemplateDialog, { props: { open: true } })
    expect(open.find('[data-testid="rider-template-dialog"]').exists()).toBe(true)
  })

  it('shows the API\'s reason when creating fails, and limits the name length up front', async () => {
    const w = mount(RiderTemplateDialog, { props: { open: true } })
    const name = w.find('[data-testid="rider-template-dialog-name"]')
    expect(name.attributes('maxlength')).toBe('200')

    fetchMock().mockRejectedValueOnce(Object.assign(new Error('x'), { statusCode: 422, data: { error: 'invalid rider input: name is required' } }))
    await name.setValue('Club')
    await w.find('[data-testid="rider-template-dialog-submit"]').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('name is required')
  })

  it('clears its fields when closed, so a reopened dialog does not invite a duplicate create', async () => {
    const w = mount(RiderTemplateDialog, { props: { open: true } })
    await w.find('[data-testid="rider-template-dialog-name"]').setValue('Standard club')
    await w.setProps({ open: false })
    await w.setProps({ open: true })
    expect((w.find('[data-testid="rider-template-dialog-name"]').element as HTMLInputElement).value).toBe('')
  })
})

describe('AttachRiderDialog', () => {
  it('loads the templates when opened with none cached, so the picker offers more than "blank"', async () => {
    fetchMock().mockResolvedValueOnce({ data: [template()] })
    const store = useRiderStore()
    mount(AttachRiderDialog, { props: { open: true, gigId: 'gig-1' } })
    await flushPromises()
    expect(urls()).toEqual(['GET /api/v1/rider/templates'])
    expect(store.templates).toHaveLength(1)
  })

  it('does not refetch templates that are already cached', async () => {
    useRiderStore().templates = [template()]
    mount(AttachRiderDialog, { props: { open: true, gigId: 'gig-1' } })
    await flushPromises()
    expect(fetchMock()).not.toHaveBeenCalled()
  })

  it('treats 409 (a rider already exists for this gig) as attached, not as an error', async () => {
    useRiderStore().templates = [template()]
    fetchMock().mockRejectedValueOnce(Object.assign(new Error('409 Conflict'), { statusCode: 409 }))
    const w = mount(AttachRiderDialog, { props: { open: true, gigId: 'gig-1' } })
    await w.find('[data-testid="attach-rider-confirm"]').trigger('click')
    await flushPromises()
    expect(w.emitted('attached')).toHaveLength(1)
    expect(w.text()).not.toContain('409')
  })

  it('shows the API\'s reason for other failures and does not report attached', async () => {
    useRiderStore().templates = [template()]
    fetchMock().mockRejectedValueOnce(Object.assign(new Error('[POST] "/api/v1/rider/attachments": 422'), {
      statusCode: 422, data: { error: 'invalid rider input: technical is too long (20001 characters; the limit is 20000)' },
    }))
    const w = mount(AttachRiderDialog, { props: { open: true, gigId: 'gig-1' } })
    await w.find('[data-testid="attach-rider-confirm"]').trigger('click')
    await flushPromises()
    expect(w.emitted('attached')).toBeUndefined()
    expect(w.text()).toContain('technical is too long')
    expect(w.text()).not.toContain('[POST]') // ofetch\'s generic message is not shown
  })

  it('falls back to a plain message when the failure carries no reason', async () => {
    useRiderStore().templates = [template()]
    fetchMock().mockRejectedValueOnce(Object.assign(new Error('fetch failed'), { statusCode: 500 }))
    const w = mount(AttachRiderDialog, { props: { open: true, gigId: 'gig-1' } })
    await w.find('[data-testid="attach-rider-confirm"]').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Could not attach the rider.')
  })
})

describe('RiderAttachmentCard', () => {
  it('loads the gig\'s attachment itself instead of claiming "NO RIDER ATTACHED"', async () => {
    fetchMock()
      .mockResolvedValueOnce({ data: attachment() })
      .mockResolvedValueOnce({ data: [template()] })
    const w = mount(RiderAttachmentCard, { props: { gig } })
    await flushPromises()
    expect(urls()).toEqual(['GET /api/v1/rider/attachments/by-gig/gig-1', 'GET /api/v1/rider/templates'])
    expect(w.find('[data-testid="rider-status-gig-1"]').text()).toBe('FROM STANDARD CLUB')
  })

  it('shows no "Invalid Date" line for an id-only gig (the /rider deep link), and shows date and venue for a full gig', async () => {
    fetchMock().mockResolvedValueOnce(null)
    const bare = mount(RiderAttachmentCard, { props: { gig } })
    await flushPromises()
    expect(bare.text()).not.toContain('Invalid Date')
    expect(bare.text()).not.toContain('VENUE TBA')
    expect(bare.text()).toContain('NO RIDER ATTACHED')

    fetchMock().mockResolvedValueOnce(null)
    const full = mount(RiderAttachmentCard, { props: { gig: { id: 'gig-2', date: '2026-10-04', venue: 'Tresor' } as Gig } })
    await flushPromises()
    expect(full.text()).toContain('TRESOR')
  })

  it('does not refetch a gig that is already cached', async () => {
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-1': null }
    mount(RiderAttachmentCard, { props: { gig } })
    await flushPromises()
    expect(fetchMock()).not.toHaveBeenCalled()
  })

  it('survives a failed load and still renders', async () => {
    fetchMock().mockRejectedValueOnce(new Error('network'))
    const w = mount(RiderAttachmentCard, { props: { gig } })
    await flushPromises()
    expect(w.find('[data-testid="rider-status-gig-1"]').text()).toBe('NO RIDER ATTACHED')
  })
})
