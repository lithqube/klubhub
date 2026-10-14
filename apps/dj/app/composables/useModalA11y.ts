import { nextTick, onBeforeUnmount, watch, type Ref } from 'vue'

// Keyboard / screen-reader behaviour for a hand-rolled modal (the rider
// dialogs are plain overlays, not Radix dialogs, because one of them opens
// inside the gig form's own overlay):
//   - focus moves into the dialog when it opens (the [data-autofocus]
//     element, else the first focusable one, else the panel itself)
//   - Tab / Shift+Tab wrap around inside the panel
//   - Escape closes
//   - focus returns to whatever had it before the dialog opened
// The panel should carry role="dialog", aria-modal and aria-labelledby.

const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function focusables(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE))
}

export function useModalA11y(isOpen: () => boolean, panel: Ref<HTMLElement | null>, close: () => void): void {
  let opener: HTMLElement | null = null
  let listening = false

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      e.stopPropagation()
      close()
      return
    }
    const root = panel.value
    if (e.key !== 'Tab' || !root) return
    const items = focusables(root)
    if (items.length === 0) {
      e.preventDefault()
      root.focus()
      return
    }
    const first = items[0]!
    const last = items[items.length - 1]!
    const active = document.activeElement
    const inside = active instanceof Node && root.contains(active)
    if (e.shiftKey && (!inside || active === first)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (!inside || active === last)) {
      e.preventDefault()
      first.focus()
    }
  }

  function teardown(): void {
    if (listening) document.removeEventListener('keydown', onKeydown, true)
    listening = false
    opener?.focus({ preventScroll: true })
    opener = null
  }

  watch(isOpen, async (open) => {
    if (open) {
      if (listening) return
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
      document.addEventListener('keydown', onKeydown, true)
      listening = true
      await nextTick()
      const root = panel.value
      if (!root) return
      const target = root.querySelector<HTMLElement>('[data-autofocus]') ?? focusables(root)[0] ?? root
      target.focus({ preventScroll: true })
    } else if (listening) {
      teardown()
    }
  }, { immediate: true, flush: 'post' })

  onBeforeUnmount(() => {
    if (listening) teardown()
  })
}
