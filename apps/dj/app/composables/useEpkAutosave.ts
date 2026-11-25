import { useEpkStore } from '../stores/epk';
import type { UpdateEPKContentRequest } from '../types/epk';

// Module-level so every caller shares one queue. Patches are MERGED until the
// timer fires: the previous version kept only the latest patch, so editing two
// fields within the debounce window silently dropped the first edit, and the
// server response then overwrote it locally as well.
let timer: ReturnType<typeof setTimeout> | null = null;
let pending: UpdateEPKContentRequest = {};
// Tail of the save chain; never rejects. Saves run one at a time, so an older
// request can never land after a newer one.
let chain: Promise<void> = Promise.resolve();

export function useEpkAutosave(delayMs = 1500) {
  const store = useEpkStore();

  async function send(): Promise<void> {
    // Taken only when it is this save's turn, so edits made while an earlier
    // save was in flight are coalesced into this one.
    const patch = pending;
    if (Object.keys(patch).length === 0) return;
    pending = {};
    try {
      await store.updateContent(patch);
      if (Object.keys(pending).length > 0) store.saveStatus = 'saving';
    } catch {
      // Keep the failed edit for the next save; anything typed since wins.
      // store.saveStatus is 'error'; the page surfaces it.
      pending = { ...patch, ...pending };
    }
  }

  function flush(): Promise<void> {
    if (timer) clearTimeout(timer);
    timer = null;
    chain = chain.then(send);
    return chain;
  }

  function scheduleSave(patch: UpdateEPKContentRequest): void {
    pending = { ...pending, ...patch };
    store.saveStatus = 'saving';
    if (timer) clearTimeout(timer);
    timer = setTimeout(flush, delayMs);
  }

  return { scheduleSave, flush };
}
