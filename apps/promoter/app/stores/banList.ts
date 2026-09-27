import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import type { ApiError } from '~/types/event'
import type { BanEntry, BanInput, BanListResponse, BanPlain, BanRecord } from '~/types/sealed'
import { apiFetch, toApiError } from '~/utils/api'
import { sortBan, tidyPlain } from '~/utils/sealed/ban'
import { useSealedStore } from './sealed'

/**
 * The ban list (P2.6): fetched as ciphertext, decrypted in the browser with
 * the OSK the sealed store holds, kept in memory while unlocked and dropped
 * when the keys lock. Writes encrypt first; ids are client uuids (they are
 * part of each entry's AAD). A write against a rotated key answers 409
 * key_version_stale: the page offers RELOAD AND RETRY (refreshStale()
 * follows the new key version, then the same write is sent again).
 */
export const useBanListStore = defineStore('banList', () => {
  const sealed = useSealedStore()

  const entries = ref<BanEntry[]>([])
  const keyVersion = ref<number | null>(null)
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref<ApiError | null>(null)

  const sorted = computed(() => sortBan(entries.value))
  const unreadable = computed(() => entries.value.filter(e => !e.plain).length)

  async function toEntry(r: BanRecord): Promise<BanEntry> {
    return {
      id: r.id, key_version: r.key_version, expires_at: r.expires_at, created_at: r.created_at, updated_at: r.updated_at,
      plain: await sealed.openEntry(r.id, r.key_version, r.entry_sealed),
    }
  }

  function clear() {
    entries.value = []
    keyVersion.value = null
    loaded.value = false
  }

  /** Load and decrypt (nothing while locked). Never throws (error is kept). */
  async function load(): Promise<void> {
    if (!sealed.unlocked) return clear()
    loading.value = true
    error.value = null
    try {
      const r = await apiFetch<BanListResponse>('/api/v1/ban-list')
      keyVersion.value = r.key_version
      entries.value = await Promise.all((r.entries ?? []).map(toEntry))
      loaded.value = true
    } catch (e) {
      error.value = toApiError(e)
    } finally {
      loading.value = false
    }
  }

  async function send<T>(fn: () => Promise<T>): Promise<T> {
    if (!sealed.unlocked) throw { error: 'locked' } as ApiError
    try {
      return await fn()
    } catch (e) {
      throw (e as ApiError)?.error && !(e as { data?: unknown }).data ? e : toApiError(e)
    }
  }

  function upsert(e: BanEntry) {
    const i = entries.value.findIndex(x => x.id === e.id)
    entries.value = i < 0 ? [...entries.value, e] : entries.value.map(x => (x.id === e.id ? e : x))
  }

  /** Add an entry (name and reason required, checked by the form). Throws an ApiError. */
  async function add(plain: BanPlain, expiresAt: string): Promise<BanEntry> {
    return send(async () => {
      const id = globalThis.crypto.randomUUID()
      const p = tidyPlain(plain)
      const { entry_sealed, key_version } = await sealed.sealEntry(id, p)
      const body: BanInput = { id, key_version, entry_sealed, expires_at: expiresAt }
      const r = await apiFetch<Partial<BanRecord> | null>('/api/v1/ban-list', { method: 'POST', body })
      const now = new Date().toISOString()
      const e: BanEntry = { id, key_version, expires_at: r?.expires_at ?? expiresAt, created_at: r?.created_at ?? now, updated_at: r?.updated_at ?? now, plain: p }
      upsert(e)
      return e
    })
  }

  /** Change an entry (re-encrypted under the current key). Throws an ApiError. */
  async function update(id: string, plain: BanPlain, expiresAt: string): Promise<BanEntry> {
    return send(async () => {
      const p = tidyPlain(plain)
      const { entry_sealed, key_version } = await sealed.sealEntry(id, p)
      const body: BanInput = { id, key_version, entry_sealed, expires_at: expiresAt }
      const r = await apiFetch<Partial<BanRecord> | null>(`/api/v1/ban-list/${id}`, { method: 'PUT', body })
      const prev = entries.value.find(x => x.id === id)
      const now = new Date().toISOString()
      const e: BanEntry = {
        id, key_version, expires_at: r?.expires_at ?? expiresAt, created_at: r?.created_at ?? prev?.created_at ?? now, updated_at: r?.updated_at ?? now, plain: p,
      }
      upsert(e)
      return e
    })
  }

  /** Remove an entry. Throws an ApiError (a missing entry is already gone: removed locally too). */
  async function remove(id: string): Promise<void> {
    try {
      await send(() => apiFetch(`/api/v1/ban-list/${id}`, { method: 'DELETE' }))
    } catch (e) {
      if ((e as ApiError).status !== 404) throw e
    }
    entries.value = entries.value.filter(x => x.id !== id)
  }

  /**
   * After key_version_stale: follow the new key version (the member key in
   * memory opens the new wrap) and reload. False when the keys had to lock.
   */
  async function refreshStale(): Promise<boolean> {
    const ok = await sealed.refreshKeys()
    await load()
    return ok
  }

  watch(() => sealed.unlockedVersion, (v, was) => {
    if (v === null) clear()
    else if (was !== null && v !== was && loaded.value) void load()
  })

  return { entries, sorted, unreadable, keyVersion, loaded, loading, error, load, add, update, remove, refreshStale, clear }
})
