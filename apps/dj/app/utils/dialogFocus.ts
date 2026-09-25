// Focus handling for controlled dialogs. Radix only restores focus to a
// DialogTrigger; our dialogs and sheets are opened from the store or from
// buttons elsewhere, so they remember the opener themselves and give it
// focus back on close. On open, focus goes to the title (tabindex="-1") so
// screen readers announce where the user landed.

import { watch, type Ref, type WatchSource } from 'vue'

type ElRef = Ref<unknown>

function elementOf(r: ElRef): HTMLElement | null {
  const v = r.value as { $el?: unknown } | HTMLElement | null
  if (!v) return null
  if (v instanceof HTMLElement) return v
  return v.$el instanceof HTMLElement ? v.$el : null
}

/** Focuses the element behind `target` once it is in the DOM (retries a few frames). */
export function focusWhenReady(target: ElRef, tries = 10): void {
  if (typeof window === 'undefined') return
  const attempt = (left: number) => {
    const el = elementOf(target)
    if (el && el.isConnected) {
      el.focus({ preventScroll: true })
      return
    }
    if (left > 0) setTimeout(() => attempt(left - 1), 16)
  }
  setTimeout(() => attempt(tries), 16)
}

/**
 * Wires "focus the title on open, restore the opener on close".
 * Returns the handlers for the dialog's open/close auto-focus events.
 */
export function useDialogFocus(open: WatchSource<boolean>, title: ElRef, refocus?: WatchSource<unknown>) {
  let opener: HTMLElement | null = null

  function restore(): void {
    const el = opener
    opener = null
    if (el && el.isConnected) el.focus({ preventScroll: true })
  }

  watch(open, (isOpen) => {
    if (typeof document === 'undefined') return
    if (isOpen) {
      const active = document.activeElement
      opener = active instanceof HTMLElement && active !== document.body ? active : null
      focusWhenReady(title)
    } else {
      // Fallback when the close auto-focus event does not fire: once the
      // dialog is gone and focus fell back to <body>, return it to the opener.
      setTimeout(() => {
        const active = document.activeElement
        if (opener && (!active || active === document.body)) restore()
      }, 60)
    }
  })

  if (refocus) {
    watch(refocus, () => focusWhenReady(title))
  }

  function onOpenAutoFocus(e: Event): void {
    e.preventDefault()
    focusWhenReady(title, 2)
  }

  function onCloseAutoFocus(e: Event): void {
    e.preventDefault()
    restore()
  }

  return { onOpenAutoFocus, onCloseAutoFocus }
}
