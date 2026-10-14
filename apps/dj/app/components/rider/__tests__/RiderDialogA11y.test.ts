import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
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
