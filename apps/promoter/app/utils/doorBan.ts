/**
 * The ban list at the door (P2.6). The bundle's `sealed` block carries the
 * OSK wrapped to this device and the encrypted entries; the device opens
 * them offline with its own X25519 key and keeps the result in memory only
 * (the encrypted bundle stays in the door vault).
 *
 * Matching is deliberately loose and only ever says "possible match": the
 * names are accent-folded and split into tokens (utils/doorSearch
 * normalise), and an entry matches when every token of the shorter name is
 * among the longer name's tokens — but only when the shorter name has at
 * least two tokens: a one-word name ("Ole") matches only a one-word name
 * that is the same word, never every "Ole Something" on the list. The
 * bundle carries names only, so email never takes part at the door.
 */
import type { DoorSealed } from '~/types/sealed'
import { normalise } from '~/utils/doorSearch'
import { wipe } from '~/utils/sealed/bytes'
import { decryptBan } from '~/utils/sealed/ban'
import { oskAad, unwrapOsk } from '~/utils/sealed/keys'

export interface DoorBanEntry {
  id: string
  name: string
  email?: string
  reason: string
  note?: string
  expires_at: string
}

export const banTokens = (name: string): string[] => [...new Set(normalise(name).split(' ').filter(Boolean))]

/** The minimum token count of the shorter name for a subset match. */
export const MIN_SUBSET_TOKENS = 2

/**
 * Possible match: the shorter name (≥ 2 tokens) is contained in the longer
 * one, either way round; single-token names only match exactly (the same
 * one token on both sides).
 */
export function namesMatch(entryName: string, guestName: string): boolean {
  const a = banTokens(entryName)
  const b = banTokens(guestName)
  if (!a.length || !b.length) return false
  const [short, long] = a.length <= b.length ? [a, b] : [b, a]
  if (short.length < MIN_SUBSET_TOKENS) return a.length === b.length && a[0] === b[0]
  const sl = new Set(long)
  return short.every(t => sl.has(t))
}

/** A ban list name with a single token (the form warns: it only matches that exact one-word name). */
export const isSingleToken = (name: string) => banTokens(name).length === 1

/** Entries that possibly match a guest (name tokens; exact email when both have one). */
export function banMatches(entries: readonly DoorBanEntry[] | null | undefined, name: string, email?: string | null): DoorBanEntry[] {
  if (!entries?.length) return []
  const mail = (email ?? '').trim().toLowerCase()
  return entries.filter(e => namesMatch(e.name, name) || (!!mail && !!e.email && e.email.toLowerCase() === mail))
}

export interface DoorBanResult {
  entries: DoorBanEntry[]
  /** Entries that did not open (tampered, other version); never shown. */
  unreadable: number
}

/**
 * Open the bundle's sealed block with the device private key: unwrap the
 * OSK (AAD klubhub-osk|tenant|version|device|device id), decrypt every
 * entry, drop expired ones. Throws SealedError when the wrap does not open.
 */
export async function openDoorBan(sealed: DoorSealed, devicePrivateKey: Uint8Array, tenant: string, deviceId: string, now = Date.now()): Promise<DoorBanResult> {
  const osk = await unwrapOsk(devicePrivateKey, sealed.wrap, oskAad(tenant, sealed.key_version, 'device', deviceId))
  const entries: DoorBanEntry[] = []
  let unreadable = 0
  try {
    for (const e of sealed.ban_entries ?? []) {
      if (Date.parse(e.expires_at) <= now) continue
      try {
        const p = await decryptBan(osk, tenant, e.id, sealed.key_version, e.entry_sealed)
        entries.push({ id: e.id, ...p, expires_at: e.expires_at })
      } catch {
        unreadable++
      }
    }
  } finally {
    wipe(osk)
  }
  return { entries, unreadable }
}
