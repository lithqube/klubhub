/**
 * The manager-PIN reveal of a ban list reason at the door (P2.6), as pure
 * rules so they can be tested without a card:
 *
 * - After REVEAL_MAX_TRIES wrong PINs in a row, SHOW REASON is disabled for
 *   REVEAL_LOCK_MS on this device (not per card: a guest cannot reset it
 *   by opening another name). A right PIN resets the count.
 * - A revealed reason hides itself after REVEAL_HIDE_MS, so it does not stay
 *   on a phone that is handed around or turned towards the queue.
 */
export const REVEAL_MAX_TRIES = 5
export const REVEAL_LOCK_MS = 60_000
export const REVEAL_HIDE_MS = 30_000

export interface RevealGuard {
  /** Wrong PINs since the last right one (or the last lockout). */
  fails: number
  /** Epoch ms until which SHOW REASON is disabled; null when not locked. */
  lockedUntil: number | null
}

export const freshGuard = (): RevealGuard => ({ fails: 0, lockedUntil: null })

/** One wrong PIN: the fifth in a row locks for a minute (and starts a new count). */
export function revealFailed(g: RevealGuard, now: number): RevealGuard {
  const fails = (revealLocked(g, now) ? 0 : g.fails) + 1
  return fails >= REVEAL_MAX_TRIES ? { fails: 0, lockedUntil: now + REVEAL_LOCK_MS } : { fails, lockedUntil: null }
}

/** The right PIN: the count starts again. */
export const revealSucceeded = (): RevealGuard => freshGuard()

export const revealLocked = (g: RevealGuard, now: number): boolean => g.lockedUntil !== null && now < g.lockedUntil

/** Whole seconds until SHOW REASON works again (0 when not locked). */
export function lockSecondsLeft(g: RevealGuard, now: number): number {
  return revealLocked(g, now) ? Math.ceil((g.lockedUntil! - now) / 1000) : 0
}

/** Tries left before the lockout (for the error line). */
export const triesLeft = (g: RevealGuard) => Math.max(0, REVEAL_MAX_TRIES - g.fails)
