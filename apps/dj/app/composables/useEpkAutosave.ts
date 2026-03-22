import { useEpkStore } from '../stores/epk';
import type { UpdateEPKContentRequest } from '../types/epk';

export function useEpkAutosave(delayMs = 1500) {
  const store = useEpkStore();
  let timer: ReturnType<typeof setTimeout> | null = null;

  function scheduleSave(patch: UpdateEPKContentRequest): void {
    store.saveStatus = 'saving';
    if (timer) clearTimeout(timer);
    timer = setTimeout(async () => {
      await store.updateContent(patch);
    }, delayMs);
  }

  return { scheduleSave };
}
