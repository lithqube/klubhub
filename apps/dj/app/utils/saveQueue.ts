/**
 * Saves that never lose or reorder an edit: patches merge until they are sent,
 * run one at a time, and a failed patch goes back under any newer values.
 */
export function createSaveQueue<P extends object>(save: (patch: P) => Promise<unknown>) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let pending = {} as P
  // Tail of the save chain; never rejects.
  let chain: Promise<void> = Promise.resolve()

  const hasPending = () => Object.keys(pending).length > 0

  async function send(): Promise<void> {
    // Taken only when it is this save's turn, so edits made while an earlier
    // save was in flight are coalesced into this one, newest values winning.
    if (!hasPending()) return
    const patch = pending
    pending = {} as P
    try {
      await save(patch)
    } catch {
      // Keep the failed edit for the next save; the store reports the error.
      pending = { ...patch, ...pending }
    }
  }

  /** Sends what is queued now. Resolves once every save started so far is done. */
  function flush(): Promise<void> {
    if (timer) clearTimeout(timer)
    timer = null
    chain = chain.then(send)
    return chain
  }

  /** Queues a patch and (re)starts the debounce. */
  function push(patch: P, delayMs: number): void {
    pending = { ...pending, ...patch }
    if (timer) clearTimeout(timer)
    timer = setTimeout(flush, delayMs)
  }

  return { push, flush, hasPending }
}
