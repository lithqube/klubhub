import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RiderTemplateCreateForm from '../RiderTemplateCreateForm.vue'

// Real Pinia store, stubbed transport. The form is inline (no dialog shell), so
// nothing here is stubbed except the network.

const created = { id: 't9', name: 'Fresh', technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: 'T', updatedAt: 'T' }
type Call = [string, { method?: string; body?: Record<string, unknown> }?]
const fetchMock = () => vi.mocked($fetch as unknown as (url: string, o?: Call[1]) => Promise<unknown>)

let w: VueWrapper | undefined
const mountForm = (props: { focusSeq?: number } = {}) => {
  w = mount(RiderTemplateCreateForm, { attachTo: document.body, props })
  return w
}
const name = () => w!.get('[data-testid="rider-template-create-name"]')
const submit = () => w!.get('[data-testid="rider-template-create-submit"]')
const cancel = () => w!.get('[data-testid="rider-template-create-cancel"]')
const escape = () => w!.get('section').trigger('keydown', { key: 'Escape' })

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: created }))
  document.body.replaceChildren()
})
afterEach(() => {
  w?.unmount()
  w = undefined
  vi.unstubAllGlobals()
})

describe('RiderTemplateCreateForm (inline)', () => {
  it('is a section in the page, not a dialog, named by its heading', () => {
    mountForm()
    expect(w!.find('[role="dialog"]').exists()).toBe(false)
    expect(w!.find('[aria-modal]').exists()).toBe(false)
    const section = w!.get('section')
    const heading = document.getElementById(section.attributes('aria-labelledby')!)
    expect(heading?.textContent).toBe('NEW TEMPLATE')
  })

  it('labels the name field and limits its length up front', () => {
    mountForm()
    const input = name().element as HTMLInputElement
    expect(document.querySelector(`label[for="${input.id}"]`)?.textContent).toBe('TEMPLATE NAME')
    expect(name().attributes('maxlength')).toBe('200')
  })

  it('moves focus to the name when it opens and when asked again', async () => {
    vi.useFakeTimers()
    try {
      mountForm({ focusSeq: 1 })
      await vi.advanceTimersByTimeAsync(100)
      expect(document.activeElement).toBe(name().element)
      ;(document.activeElement as HTMLElement).blur()
      await w!.setProps({ focusSeq: 2 })
      await vi.advanceTimersByTimeAsync(100)
      expect(document.activeElement).toBe(name().element)
    } finally {
      vi.useRealTimers()
    }
  })

  it('creates the template with all four sections, then reports it and closes', async () => {
    mountForm()
    await name().setValue('  Fresh  ')
    await w!.get('[data-testid="rider-technical-textarea"]').setValue('2x CDJ')
    await w!.get('[data-testid="rider-hospitality-textarea"]').setValue('water')
    await w!.get('[data-testid="rider-backline-textarea"]').setValue('monitors')
    await w!.get('[data-testid="rider-otherNotes-textarea"]').setValue('parking')
    await submit().trigger('click')
    await flushPromises()

    expect(fetchMock()).toHaveBeenCalledWith('/api/v1/rider/templates', expect.objectContaining({
      method: 'POST',
      body: { name: 'Fresh', technical: '2x CDJ', hospitality: 'water', backline: 'monitors', otherNotes: 'parking' },
    }))
    expect(w!.emitted('created')?.[0]).toEqual([created])
    expect(w!.emitted('close')).toHaveLength(1)
  })

  it('submits with Enter in the name field', async () => {
    mountForm()
    await name().setValue('Club')
    await name().trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(fetchMock()).toHaveBeenCalledTimes(1)
    expect(w!.emitted('created')).toHaveLength(1)
  })

  it('asks for a name instead of sending an empty one', async () => {
    mountForm()
    await submit().trigger('click')
    await flushPromises()
    expect(fetchMock()).not.toHaveBeenCalled()
    expect(w!.get('[role="alert"]').text()).toContain('Name is required')
    expect(name().attributes('aria-invalid')).toBe('true')
    expect(w!.emitted('close')).toBeUndefined()
  })

  it('shows the API\'s reason when creating fails and stays open with the text intact', async () => {
    fetchMock().mockRejectedValueOnce(Object.assign(new Error('x'), { statusCode: 422, data: { error: 'invalid rider input: name is required' } }))
    mountForm()
    await name().setValue('Club')
    await w!.get('[data-testid="rider-technical-textarea"]').setValue('keep me')
    await submit().trigger('click')
    await flushPromises()

    expect(w!.get('[role="alert"]').text()).toContain('name is required')
    expect(w!.emitted('close')).toBeUndefined()
    expect((w!.get('[data-testid="rider-technical-textarea"]').element as HTMLTextAreaElement).value).toBe('keep me')
    expect(submit().attributes('disabled')).toBeUndefined() // can try again
  })

  it('does not create twice while the first request is pending', async () => {
    let resolve!: (v: unknown) => void
    fetchMock().mockReturnValueOnce(new Promise((r) => { resolve = r }))
    mountForm()
    await name().setValue('Club')
    await submit().trigger('click')
    await name().trigger('keydown', { key: 'Enter' })
    expect(submit().attributes('disabled')).toBeDefined()
    expect(submit().text()).toBe('CREATING…')
    resolve({ data: created })
    await flushPromises()
    expect(fetchMock()).toHaveBeenCalledTimes(1)
  })

  it('CANCEL always closes, even with text typed', async () => {
    mountForm()
    await name().setValue('Half done')
    await cancel().trigger('click')
    expect(w!.emitted('close')).toHaveLength(1)
    expect(fetchMock()).not.toHaveBeenCalled()
  })

  it('Escape closes an empty form but never throws away a draft', async () => {
    mountForm()
    await escape()
    expect(w!.emitted('close')).toHaveLength(1)

    await name().setValue('Draft')
    await escape()
    expect(w!.emitted('close')).toHaveLength(1) // ignored: a name is typed

    await name().setValue('')
    await w!.get('[data-testid="rider-hospitality-textarea"]').setValue('long notes')
    await escape()
    expect(w!.emitted('close')).toHaveLength(1) // ignored: a section is typed
  })
})
