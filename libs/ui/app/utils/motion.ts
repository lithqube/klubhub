import { animate, stagger, type AnimationParams } from 'animejs'

/**
 * Kinetic HUD motion: short, state-showing movement only (DESIGN.md → Motion).
 * Every helper does nothing under reduced motion or in a hidden tab, and
 * removes its inline styles afterwards: a leftover transform would become the
 * containing block of any fixed-position modal inside the element.
 */
function canAnimate(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return !document.hidden && !window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function clean(els: HTMLElement[]) {
  for (const el of els) {
    for (const prop of ['opacity', 'transform', 'translate', 'scale']) el.style.removeProperty(prop)
  }
}

/** Longer than any animation here, so it only fires when frames have stalled. */
const SETTLE_MS = 800

/**
 * Runs an animation and always ends clean. A throttled or occluded window can
 * stop the frame loop while the document still reports visible: without the
 * timer, content would stay faded.
 */
function play(els: HTMLElement[], params: AnimationParams) {
  const timer = setTimeout(() => settle(), SETTLE_MS)
  const animation = animate(els, { ...params, onComplete: () => settle() })
  function settle() {
    clearTimeout(timer)
    animation.cancel()
    clean(els)
  }
}

/** Marks the sections present now so revealSections leaves them alone. */
export function markSectionsShown(root: ParentNode = document): HTMLElement[] {
  const fresh = [...root.querySelectorAll<HTMLElement>('.page-body > *:not([data-kh-revealed])')]
  for (const el of fresh) el.dataset.khRevealed = ''
  return fresh
}

/** Staggered entrance for page sections that have not been shown yet. */
export function revealSections(root: ParentNode = document) {
  const fresh = markSectionsShown(root)
  if (!canAnimate()) return
  // Past the eighth section the stagger would only delay what is off screen.
  const els = fresh.slice(0, 8)
  if (!els.length) return
  play(els, { opacity: [0.2, 1], y: [8, 0], duration: 220, delay: stagger(30), ease: 'outQuad' })
}

/** A result card arriving. Starts readable, so it never delays a decision. */
export function popIn(el: HTMLElement | null) {
  if (!el || !canAnimate()) return
  play([el], { opacity: [0.6, 1], scale: [0.98, 1], duration: 160, ease: 'outQuad' })
}

/** "No": a short sideways shake for a refused or not-yet-confirmed action. */
export function shake(el: HTMLElement | null) {
  if (!el || !canAnimate()) return
  play([el], { x: [{ to: -6 }, { to: 6 }, { to: -4 }, { to: 4 }, { to: 0 }], duration: 260, ease: 'linear' })
}
