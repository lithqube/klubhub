import { describe, expect, it } from 'vitest'
import {
  freshGuard, lockSecondsLeft, REVEAL_HIDE_MS, REVEAL_LOCK_MS, REVEAL_MAX_TRIES, revealFailed, revealLocked, revealSucceeded, triesLeft,
} from '../doorReveal'

describe('manager PIN reveal guard', () => {
  it('locks SHOW REASON for 60 s after 5 wrong PINs in a row', () => {
    let g = freshGuard()
    const t0 = 1_000_000
    for (let i = 1; i < REVEAL_MAX_TRIES; i++) {
      g = revealFailed(g, t0 + i)
      expect(revealLocked(g, t0 + i)).toBe(false)
      expect(triesLeft(g)).toBe(REVEAL_MAX_TRIES - i)
    }
    g = revealFailed(g, t0 + 10)
    expect(revealLocked(g, t0 + 10)).toBe(true)
    expect(lockSecondsLeft(g, t0 + 10)).toBe(60)
    expect(lockSecondsLeft(g, t0 + 10 + 59_001)).toBe(1)
    expect(revealLocked(g, t0 + 10 + REVEAL_LOCK_MS - 1)).toBe(true)
    expect(revealLocked(g, t0 + 10 + REVEAL_LOCK_MS)).toBe(false)
    expect(lockSecondsLeft(g, t0 + 10 + REVEAL_LOCK_MS)).toBe(0)
    // After the lockout, the count starts again from zero.
    g = revealFailed(g, t0 + 10 + REVEAL_LOCK_MS + 1)
    expect(g).toEqual({ fails: 1, lockedUntil: null })
  })

  it('resets the count on the right PIN', () => {
    let g = revealFailed(revealFailed(freshGuard(), 1), 2)
    expect(g.fails).toBe(2)
    g = revealSucceeded()
    expect(g).toEqual({ fails: 0, lockedUntil: null })
    expect(triesLeft(g)).toBe(REVEAL_MAX_TRIES)
  })

  it('hides a revealed reason after 30 s', () => {
    expect(REVEAL_HIDE_MS).toBe(30_000)
  })
})
