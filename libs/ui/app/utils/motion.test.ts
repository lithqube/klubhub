import { afterEach, describe, expect, it, vi } from 'vitest'
import { popIn, revealSections, shake } from './motion'

// A frame loop that never advances: the start state is applied and nothing else.
const cancel = vi.fn()
vi.mock('animejs', () => ({
  stagger: () => 0,
  animate: vi.fn((els: HTMLElement[]) => {
    for (const el of els) el.style.opacity = '0.2'
    return { cancel }
  }),
}))

function setup(reduced: boolean) {
  window.matchMedia = vi.fn().mockReturnValue({ matches: reduced }) as unknown as typeof window.matchMedia
  document.body.innerHTML = '<div class="page-body"><section></section><section></section></div>'
  return [...document.querySelectorAll<HTMLElement>('section')]
}

describe('motion', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.useRealTimers()
  })

  it('leaves everything untouched under reduced motion', () => {
    const els = setup(true)
    revealSections()
    popIn(els[0]!)
    shake(els[1]!)
    for (const el of els) expect(el.getAttribute('style')).toBeNull()
  })

  it('never leaves content faded when frames stall, and reveals a section once', () => {
    vi.useFakeTimers()
    const els = setup(false)
    revealSections()
    expect(els[0]!.style.opacity).toBe('0.2')
    vi.advanceTimersByTime(800)
    expect(cancel).toHaveBeenCalled()
    for (const el of els) expect(el.style.opacity).toBe('')
    revealSections()
    expect(els[0]!.style.opacity).toBe('')
  })
})
