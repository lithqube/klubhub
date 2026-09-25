// Demo persistence: one versioned localStorage key, memory fallback when
// storage is unavailable (private mode, quota, blocked site data).

import type { DemoState } from './types'

export const STORAGE_KEY = 'klubhub-demo:v1'

/** Subset of the Web Storage API the demo needs. */
export interface KeyValueStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export function browserStorage(): KeyValueStorage | null {
  try {
    const s = globalThis.localStorage
    if (!s) return null
    const probe = `${STORAGE_KEY}:probe`
    s.setItem(probe, '1')
    s.removeItem(probe)
    return s
  } catch {
    return null
  }
}

export class DemoStore {
  state: DemoState
  /** False once a write failed; changes then live in memory only. */
  persistent: boolean

  constructor(
    private readonly storage: KeyValueStorage | null,
    private readonly seed: () => DemoState,
  ) {
    this.persistent = storage !== null
    this.state = this.read() ?? this.fresh()
  }

  private read(): DemoState | null {
    if (!this.storage) return null
    try {
      const raw = this.storage.getItem(STORAGE_KEY)
      if (!raw) return null
      const parsed = JSON.parse(raw) as DemoState
      return parsed && parsed.version === 1 ? parsed : null
    } catch {
      return null
    }
  }

  private fresh(): DemoState {
    const s = this.seed()
    this.write(s)
    return s
  }

  private write(s: DemoState): void {
    if (!this.storage) return
    try {
      this.storage.setItem(STORAGE_KEY, JSON.stringify(s))
      this.persistent = true
    } catch {
      this.persistent = false
    }
  }

  save(): void {
    this.write(this.state)
  }

  /** Drops all changes and re-seeds. */
  reset(): DemoState {
    try {
      this.storage?.removeItem(STORAGE_KEY)
    } catch {
      // ignore: the fresh write below replaces it anyway
    }
    this.state = this.fresh()
    return this.state
  }
}
