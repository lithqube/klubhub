// Mock state shared across all rider/* Nitro endpoints so CRUD appears to
// persist for the lifetime of the dev server (reset on restart). The rules
// (update tokens, limits, unique names, ...) live in the shared RiderMockDb,
// the same implementation the browser demo uses.

import { RiderMockDb, type Result } from '../../../../shared/rider-mock/db'

export const db = new RiderMockDb({ seed: true })

/** Returns the value of a successful db call; throws the matching Nitro error otherwise. */
export function unwrap<T>(result: Result<T>): T {
  if (!result.ok) throw createError({ statusCode: result.status, statusMessage: result.message })
  return result.value
}
