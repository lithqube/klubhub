import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RiderTemplateDialog from '../RiderTemplateDialog.vue'
import AttachRiderDialog from '../AttachRiderDialog.vue'
import { useRiderStore } from '../../../stores/rider'

// The rider dialogs are plain overlays, so they must supply what Radix would:
// a name for screen readers, focus moved in, a Tab trap, Escape, focus back.

let w: VueWrapper | undefined
beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: [] }))
  document.body.replaceChildren()
})
afterEach(() => {
  w?.unmount()
  w = undefined
  vi.unstubAllGlobals()
})

const press = (key: string, opts: KeyboardEventInit = {}) =>
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...opts }))
const active = () => document.activeElement as HTMLElement | null

describe('RiderTemplateDialog accessibility', () => {
  const open = async (props: { open: boolean } = { open: true }) => {
    w = mount(RiderTemplateDialog, { attachTo: document.body, props })
    await flushPromises()
    return w
  }

  it('is named by its heading and its name field has a real label', async () => {
    await open()
    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    const heading = document.getElementById(dialog.getAttribute('aria-labelledby')!)
    expect(heading?.textContent).toContain('NEW TEMPLATE')

    const input = document.querySelector('[data-testid="rider-template-dialog-name"]') as HTMLInputElement
    const label = document.querySelector(`label[for="${input.id}"]`)
    expect(label?.textContent).toBe('NAME')
  })

  it('the close button has an accessible name', async () => {
    await open()
    expect(document.querySelector('button[aria-label="Close"]')).not.toBeNull()
  })

  it('moves focus into the dialog (the name field) when it opens', async () => {
    await open()
    expect(active()?.getAttribute('data-testid')).toBe('rider-template-dialog-name')
  })

  it('Escape closes it', async () => {
    const wrapper = await open()
    press('Escape')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('does nothing on Escape while closed (no stray listener)', async () => {
    const wrapper = await open({ open: false })
    press('Escape')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('Tab wraps from the last control to the first, Shift+Tab from the first to the last', async () => {
    await open()
    const panel = document.querySelector('[role="dialog"] > div') as HTMLElement
    const controls = Array.from(panel.querySelectorAll<HTMLElement>('button:not([disabled]), input, textarea'))
    const first = controls[0]!
    const last = controls[controls.length - 1]!

    last.focus()
    press('Tab')
    expect(active()).toBe(first)

    first.focus()
    press('Tab', { shiftKey: true })
    expect(active()).toBe(last)
  })

  it('focus does not escape to the page behind: Tab from outside returns inside', async () => {
    await open()
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    outside.focus()
    press('Tab')
    expect(document.querySelector('[role="dialog"]')!.contains(active())).toBe(true)
  })

  it('gives focus back to what had it before it opened', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()

    const wrapper = await open({ open: false })
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(opener.contains(active())).toBe(false) // focus moved in

    await wrapper.setProps({ open: false })
    await flushPromises()
    expect(active()).toBe(opener)
  })

  it('reports the created template so the page can select it', async () => {
    const created = { id: 't9', name: 'Fresh', technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: 'T', updatedAt: 'T' }
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: created }))
    const wrapper = await open()
    await wrapper.find('[data-testid="rider-template-dialog-name"]').setValue('Fresh')
    await wrapper.find('[data-testid="rider-template-dialog-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('created')?.[0]).toEqual([created])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})

describe('AttachRiderDialog accessibility', () => {
  const tpl = { id: 't1', name: 'Club', technical: 'x', hospitality: '', backline: '', otherNotes: '', createdAt: 'T', updatedAt: 'T' }
  const open = async () => {
    useRiderStore().templates = [tpl]
    w = mount(AttachRiderDialog, { attachTo: document.body, props: { open: true, gigId: 'g1' } })
    await flushPromises()
    return w
  }

  it('is named by its heading and the close button has a name', async () => {
    await open()
    const dialog = document.querySelector('[role="dialog"]')!
    expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toContain('ATTACH RIDER')
    expect(document.querySelector('button[aria-label="Close"]')).not.toBeNull()
  })

  it('moves focus in and Escape closes', async () => {
    const wrapper = await open()
    expect(document.querySelector('[role="dialog"]')!.contains(active())).toBe(true)
    press('Escape')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('the choice is exposed to assistive tech (aria-pressed follows the selection)', async () => {
    const wrapper = await open()
    const blank = wrapper.find('[data-testid="attach-rider-blank"]')
    const club = wrapper.find('[data-testid="attach-rider-template-t1"]')
    expect(blank.attributes('aria-pressed')).toBe('true')
    expect(club.attributes('aria-pressed')).toBe('false')

    await club.trigger('click')
    expect(blank.attributes('aria-pressed')).toBe('false')
    expect(club.attributes('aria-pressed')).toBe('true')
  })

  it('an attach failure is announced', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(Object.assign(new Error('x'), { statusCode: 500, data: { message: 'storage is down' } })))
    const wrapper = await open()
    await wrapper.find('[data-testid="attach-rider-confirm"]').trigger('click')
    await flushPromises()
    const alert = wrapper.find('[role="alert"]')
    expect(alert.exists()).toBe(true)
    expect(alert.text()).toContain('storage is down')
  })
})
