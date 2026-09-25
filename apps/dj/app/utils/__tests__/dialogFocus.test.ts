import { describe, it, expect } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useDialogFocus } from '../dialogFocus'

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

describe('useDialogFocus', () => {
  it('focuses the title on open and gives focus back to the opener on close', async () => {
    document.body.innerHTML = '<button id="opener">open</button><h2 id="title" tabindex="-1">INV</h2>'
    const opener = document.getElementById('opener')!
    const title = ref<HTMLElement | null>(document.getElementById('title'))
    const open = ref(false)
    const scope = effectScope()
    const handlers = scope.run(() => useDialogFocus(open, title))!

    opener.focus()
    open.value = true
    await nextTick()
    await wait(40)
    expect(document.activeElement?.id).toBe('title')

    open.value = false
    await nextTick()
    const e = new Event('close', { cancelable: true })
    handlers.onCloseAutoFocus(e)
    expect(e.defaultPrevented).toBe(true)
    expect(document.activeElement?.id).toBe('opener')
    scope.stop()
  })

  it('falls back to the opener when no close event arrives', async () => {
    document.body.innerHTML = '<button id="opener">open</button><h2 id="title" tabindex="-1">INV</h2>'
    const opener = document.getElementById('opener')!
    const title = ref<HTMLElement | null>(document.getElementById('title'))
    const open = ref(false)
    const scope = effectScope()
    scope.run(() => useDialogFocus(open, title))
    opener.focus()
    open.value = true
    await nextTick()
    await wait(40)
    ;(document.activeElement as HTMLElement).blur()
    open.value = false
    await nextTick()
    await wait(100)
    expect(document.activeElement?.id).toBe('opener')
    scope.stop()
  })
})
