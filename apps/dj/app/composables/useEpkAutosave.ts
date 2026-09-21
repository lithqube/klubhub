import { useEpkStore } from '../stores/epk';
import type { UpdateEPKContentRequest } from '../types/epk';

// Module-level so every caller shares one queue. Patches are MERGED until the
// timer fires: the previous version kept only the latest patch, so editing two
// fields within the debounce window silently dropped the first edit, and the
// server response then overwrote it locally as well.
let timer: ReturnType<typeof setTimeout> | null = null;
let pending: UpdateEPKContentRequest = {};

export function useEpkAutosave(delayMs = 1500) {
  const store = useEpkStore();

  async function flush(): Promise<void> {
    if (timer) clearTimeout(timer);
    timer = null;
    const patch = pending;
    pending = {};
    if (Object.keys(patch).length === 0) return;
    try {
      await store.updateContent(patch);
    } catch {
      // store.saveStatus is 'error'; the page surfaces it
    }
  }

  function scheduleSave(patch: UpdateEPKContentRequest): void {
    pending = { ...pending, ...patch };
    store.saveStatus = 'saving';
    if (timer) clearTimeout(timer);
    timer = setTimeout(flush, delayMs);
  }

  return { scheduleSave, flush };
}
