import { describe, expect, it } from 'vitest'
import { createSaveQueue } from '../saveQueue'

/** A save that finishes only when the test says so. */
function controlled() {
  const calls: { patch: Record<string, string>, ok: () => void, fail: () => void }[] = []
  const queue = createSaveQueue<Record<string, string>>(patch => new Promise<void>((resolve, reject) => {
    calls.push({ patch, ok: () => resolve(), fail: () => reject(new Error('offline')) })
  }))
  return { calls, queue }
}
const tick = () => new Promise(resolve => setTimeout(resolve, 0))

describe('createSaveQueue', () => {
  it('lets a correction queued during a failing save win over the restored value', async () => {
    const { calls, queue } = controlled()
    queue.push({ name: 'old' }, 10_000)
    const first = queue.flush()
    await tick()
    queue.push({ name: 'corrected' }, 10_000)
    const second = queue.flush()
    await tick()
    expect(calls).toHaveLength(1) // one at a time

    calls[0]!.fail()
    await first
    await tick()
    expect(calls[1]!.patch).toEqual({ name: 'corrected' })
    calls[1]!.ok()
    await second
    expect(queue.hasPending()).toBe(false)
  })

  it('does not resolve flush before a save already in flight has finished', async () => {
    const { calls, queue } = controlled()
    queue.push({ name: 'a' }, 10_000)
    void queue.flush()
    await tick()
    let done = false
    const waiting = queue.flush().then(() => { done = true }) // nothing new queued
    await tick()
    expect(done).toBe(false)
    calls[0]!.ok()
    await waiting
    expect(done).toBe(true)
  })
})
