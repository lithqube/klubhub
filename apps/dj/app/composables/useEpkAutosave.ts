import { useEpkStore } from '../stores/epk';
import { createSaveQueue } from '../utils/saveQueue';
import type { UpdateEPKContentRequest } from '../types/epk';

// Module-level so every caller shares one queue. Patches are MERGED until the
// timer fires: the previous version kept only the latest patch, so editing two
// fields within the debounce window silently dropped the first edit, and the
// server response then overwrote it locally as well.
const queue = createSaveQueue<UpdateEPKContentRequest>(async (patch) => {
  const store = useEpkStore();
  await store.updateContent(patch); // a failure leaves saveStatus 'error'
  if (queue.hasPending()) store.saveStatus = 'saving';
});

export function useEpkAutosave(delayMs = 1500) {
  const store = useEpkStore();

  function scheduleSave(patch: UpdateEPKContentRequest): void {
    store.saveStatus = 'saving';
    queue.push(patch, delayMs);
  }

  return { scheduleSave, flush: queue.flush };
}
