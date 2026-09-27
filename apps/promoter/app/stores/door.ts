import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import type { ApiError } from '~/types/event'
import type {
  AddsResponse, CounterKind, CounterOp, DoorAdd, DoorBundle, DoorDevice, DoorDeviceRecord, DoorOp, DoorRejection, DoorSubject,
  JournalEntry, PinInput, PinResult, PinStatus, RegisteredDevice, SyncResponse,
} from '~/types/door'
import { apiFetch, toApiError } from '~/utils/api'
import { DoorVault, idbKV, type KV, memoryKV } from '~/utils/doorDb'
import { verifyManagerPin } from '~/utils/doorPin'
import { buildIndex, type SearchEntry } from '~/utils/doorSearch'
import {
  effectiveCheckins, effectiveCounters, expiryReason, mergeCheckins, occupancy, queueUndo, recentEntries, settle,
} from '~/utils/doorState'

/** localStorage key of this browser's door device (id, label, token, event). */
export const DEVICE_KEY = 'klubhub-door-device'
export const SYNC_EVERY_MS = 15_000
const MAX_OPS = 500
const MAX_ADDS = 50
const MAX_JOURNAL = 300

export type DoorPhase = 'boot' | 'unprepared' | 'locked' | 'loading' | 'ready'

interface QueueSlot {
  ops: DoorOp[]
  adds: DoorAdd[]
  journal: JournalEntry[]
  rejections: DoorRejection[]
}

let kvFactory: () => KV = () => (typeof indexedDB === 'undefined' ? memoryKV() : idbKV())

/** Tests: swap the storage behind the door vault. */
export function setDoorKV(factory: () => KV) {
  kvFactory = factory
}

const uuid = () => globalThis.crypto.randomUUID()
const nowIso = () => new Date().toISOString()

function readDevice(): DoorDeviceRecord | null {
  try {
    const raw = globalThis.localStorage?.getItem(DEVICE_KEY)
    const d = raw ? (JSON.parse(raw) as DoorDeviceRecord) : null
    return d?.id && d.token && d.event?.id ? d : null
  } catch {
    return null
  }
}

function writeDevice(d: DoorDeviceRecord | null) {
  try {
    if (d) globalThis.localStorage?.setItem(DEVICE_KEY, JSON.stringify(d))
    else globalThis.localStorage?.removeItem(DEVICE_KEY)
  } catch {
    // Storage blocked (private mode): the device must be prepared again.
  }
}

/**
 * The door (P2.3): this browser's door device, the PIN login, the
 * encrypted offline bundle and op queue, the sync loop — plus the
 * staff-side device and PIN actions of the event DOOR tab.
 */
export const useDoorStore = defineStore('door', () => {
  const device = ref<DoorDeviceRecord | null>(null)
  const phase = ref<DoorPhase>('boot')
  const bundle = shallowRef<DoorBundle | null>(null)
  const queue = ref<DoorOp[]>([])
  const adds = ref<DoorAdd[]>([])
  const journal = ref<JournalEntry[]>([])
  const rejections = ref<DoorRejection[]>([])
  const syncing = ref(false)
  const lastSyncAt = ref<string | null>(null)
  const offline = ref(false)
  const syncError = ref<string | null>(null)
  /** The door session ended server-side (expired, revoked): log in again; the queue is kept. */
  const sessionEnded = ref(false)
  /** Why the cache was last wiped (shown on the lock screen). */
  const wipedBecause = ref<'logout' | 'session' | 'event' | null>(null)

  // Staff side (event DOOR tab).
  const devices = ref<DoorDevice[]>([])
  const pinStatus = ref<PinStatus | null>(null)

  let vault: DoorVault | null = null
  const theVault = () => (vault ??= new DoorVault(kvFactory()))
  const inflight = new Set<string>()
  let timer: ReturnType<typeof setInterval> | null = null
  let again = false

  // ---------------------------------------------------------------- derived

  const checkins = computed(() => (bundle.value ? effectiveCheckins(bundle.value.checkins, queue.value, bundle.value.device_id) : []))
  const counters = computed(() => (bundle.value ? effectiveCounters(bundle.value.counters, queue.value, journal.value) : { walkups: 0, manual_in: 0, manual_out: 0 }))
  const occ = computed(() => occupancy(checkins.value, counters.value, bundle.value?.event.capacity ?? null))
  const queued = computed(() => queue.value.length + adds.value.length)
  /** The last 10 own door actions that can still be undone (RECENT list, newest first). */
  const recent = computed(() => recentEntries(journal.value, bundle.value, 10))
  const index = computed(() => {
    const b = bundle.value
    if (!b) return []
    const entries: SearchEntry[] = [
      ...b.guests.map(g => ({ kind: 'guest' as const, id: g.id, name: g.name })),
      ...b.tickets.map(t => ({ kind: 'ticket' as const, id: t.id, name: t.name || `Order ${t.order_ref}`, order_ref: t.order_ref, secret: t.secret })),
    ]
    return buildIndex(entries)
  })

  // ---------------------------------------------------------------- persistence

  async function persistQueue() {
    if (!theVault().isOpen) return
    const slot: QueueSlot = { ops: queue.value, adds: adds.value, journal: journal.value, rejections: rejections.value }
    await theVault().put('queue', JSON.parse(JSON.stringify(slot)))
  }

  async function persistBundle() {
    if (bundle.value && theVault().isOpen) await theVault().put('bundle', bundle.value)
  }

  function reset() {
    bundle.value = null
    queue.value = []
    adds.value = []
    journal.value = []
    rejections.value = []
    lastSyncAt.value = null
    syncError.value = null
    sessionEnded.value = false
    offline.value = false
  }

  // ---------------------------------------------------------------- lifecycle

  /** Read the device record and reopen an existing door session (offline reloads). */
  async function init(): Promise<void> {
    device.value = readDevice()
    if (phase.value === 'ready' && bundle.value && bundle.value.event.id === device.value?.event.id) return
    if (!device.value) {
      phase.value = 'unprepared'
      return
    }
    const v = theVault()
    if (await v.open()) {
      const b = await v.get<DoorBundle>('bundle')
      const q = await v.get<QueueSlot>('queue')
      if (b && b.event.id === device.value.event.id) {
        const why = expiryReason(b, Date.now())
        const pending = (q?.ops.length ?? 0) + (q?.adds.length ?? 0)
        // A PIN window that ended with unsynced actions keeps them for a re-login.
        if (why && !(why === 'session' && pending)) {
          await wipe(why)
          return
        }
        bundle.value = b
        queue.value = q?.ops ?? []
        adds.value = q?.adds ?? []
        journal.value = q?.journal ?? []
        rejections.value = q?.rejections ?? []
        lastSyncAt.value = b.generated_at
        sessionEnded.value = why === 'session'
        phase.value = 'ready'
        startLoop()
        return
      }
      await v.wipe()
    }
    phase.value = 'locked'
  }

  /** PIN login, then a fresh encrypted session holding the bundle. Unsynced ops of the same event survive a re-login. */
  async function login(pin: string): Promise<void> {
    const d = device.value
    if (!d) throw { error: 'not_prepared' } as ApiError
    try {
      await apiFetch('/api/v1/door/login', { method: 'POST', body: { device_token: d.token, event_id: d.event.id, pin } })
    } catch (e) {
      throw toApiError(e)
    }
    const sameEvent = bundle.value?.event.id === d.event.id
    const carry: QueueSlot = sameEvent
      ? { ops: queue.value, adds: adds.value, journal: journal.value, rejections: rejections.value }
      : { ops: [], adds: [], journal: [], rejections: [] }
    phase.value = 'loading'
    try {
      await theVault().create()
      reset()
      queue.value = carry.ops
      adds.value = carry.adds
      journal.value = carry.journal
      rejections.value = carry.rejections
      await downloadBundle()
      wipedBecause.value = null
    } catch (e) {
      phase.value = 'locked'
      throw toApiError(e)
    }
  }

  async function downloadBundle(): Promise<void> {
    const b = await apiFetch<DoorBundle>('/api/v1/door/bundle')
    // Guests added at the door and not yet synced must stay findable.
    const pendingGuests = adds.value.filter(a => !b.guests.some(g => g.id === a.id))
      .map(a => ({ id: a.id, list_id: a.list_id, name: a.name, plus_n: a.plus_n, status: 'going' as const, note: '' }))
    bundle.value = { ...b, guests: [...b.guests, ...pendingGuests] }
    lastSyncAt.value = b.generated_at
    sessionEnded.value = false
    await persistBundle()
    await persistQueue()
    phase.value = 'ready'
    startLoop()
    if (queued.value) void sync()
  }

  /** Drop everything cached on this device (the device record stays). */
  async function wipe(reason: 'logout' | 'session' | 'event'): Promise<void> {
    stopLoop()
    await theVault().wipe()
    reset()
    wipedBecause.value = reason
    phase.value = device.value ? 'locked' : 'unprepared'
  }

  /** End the door session: sign out server-side (best effort) and wipe the cache. */
  async function logout(): Promise<void> {
    try {
      await apiFetch('/api/v1/auth/logout', { method: 'POST' })
    } catch {
      // Offline: the cookie expires with the PIN window anyway.
    }
    await wipe('logout')
  }

  /**
   * Wipe when the session or the event (+ grace) has ended. A session that
   * ends with unsynced actions is not wiped: the queue is kept and the door
   * asks for a new PIN (sessionEnded), so nothing admitted is lost.
   * Returns true when the door can no longer sync.
   */
  async function checkExpiry(now = Date.now()): Promise<boolean> {
    const why = bundle.value ? expiryReason(bundle.value, now) : null
    if (why === 'session' && queued.value) {
      sessionEnded.value = true
      return true
    }
    if (why) await wipe(why)
    return !!why
  }

  // ---------------------------------------------------------------- door ops

  function record(op: DoorOp) {
    queue.value = [...queue.value, op]
    if (op.type !== 'undo') journal.value = [...journal.value, { ...op, synced: false }].slice(-MAX_JOURNAL)
  }

  /** Check a subject in (or out); returns the op nonce for the undo toast. */
  async function checkIn(subject: DoorSubject, count: number, direction: 'in' | 'out' = 'in'): Promise<string> {
    const op: DoorOp = { nonce: uuid(), type: 'checkin', subject, count: Math.max(1, Math.min(50, Math.round(count))), direction, at: nowIso() }
    record(op)
    await persistQueue()
    void sync()
    return op.nonce
  }

  async function counter(kind: CounterKind, delta = 1): Promise<string> {
    const op: CounterOp = { nonce: uuid(), type: 'counter', kind, delta, at: nowIso() }
    record(op)
    await persistQueue()
    void sync()
    return op.nonce
  }

  async function undo(target: string): Promise<void> {
    queue.value = queueUndo(queue.value, target, inflight, uuid(), nowIso())
    // Dropped before it left the device (gone from the journal), or taken
    // back by a queued undo (kept, marked undone so RECENT stops offering it).
    const dropped = !queue.value.some(o => o.nonce === target || (o.type === 'undo' && o.target === target))
    journal.value = journal.value
      .filter(j => j.nonce !== target || j.synced || !dropped)
      .map(j => (j.nonce === target ? { ...j, undone: true } : j))
    await persistQueue()
    void sync()
  }

  /**
   * On-the-spot add: the manager PIN is checked offline against the
   * bundle's verifier, then the add and its check-in are queued. The guest
   * id is a client uuid, so the check-in can reference it before sync.
   */
  async function addGuest(input: { name: string, plus_n: number, list_id: string, manager_pin: string, count: number }): Promise<{ id: string, nonce: string }> {
    const b = bundle.value
    if (!b) throw { error: 'not_ready' } as ApiError
    if (!b.manager_pin) throw { error: 'no_manager_pin' } as ApiError
    const name = input.name.replace(/\s+/g, ' ').trim()
    if (!name || name.length > 120) throw { error: 'invalid', field: 'name' } as ApiError
    if (!b.lists.some(l => l.id === input.list_id)) throw { error: 'invalid', field: 'list_id' } as ApiError
    const plus = Math.max(0, Math.min(10, Math.round(input.plus_n) || 0))
    if (!(await verifyManagerPin(input.manager_pin, b.manager_pin))) throw { error: 'manager_pin_invalid' } as ApiError
    const add: DoorAdd = { id: uuid(), nonce: uuid(), list_id: input.list_id, name, plus_n: plus, manager_pin: input.manager_pin, at: nowIso() }
    adds.value = [...adds.value, add]
    bundle.value = { ...b, guests: [...b.guests, { id: add.id, list_id: add.list_id, name, plus_n: plus, status: 'going', note: '' }] }
    await persistBundle()
    const nonce = await checkIn({ kind: 'guest', id: add.id }, Math.min(input.count, 1 + plus))
    return { id: add.id, nonce }
  }

  function dismissRejections() {
    rejections.value = []
    void persistQueue()
  }

  // ---------------------------------------------------------------- sync

  function online() {
    return typeof navigator === 'undefined' || navigator.onLine !== false
  }

  /** Push adds (first) and ops, pull other devices' check-ins. Never throws. */
  async function sync(): Promise<void> {
    if (!bundle.value || phase.value !== 'ready' || sessionEnded.value) return
    if (syncing.value) {
      again = true
      return
    }
    if (!online()) {
      offline.value = true
      return
    }
    syncing.value = true
    again = false
    try {
      if (adds.value.length) await pushAdds()
      const ops = queue.value.slice(0, MAX_OPS)
      ops.forEach(o => inflight.add(o.nonce))
      const r = await apiFetch<SyncResponse>('/api/v1/door/checkins', { method: 'POST', body: { since: bundle.value.cursor || null, ops } })
      const s = settle(queue.value, r.results)
      queue.value = s.queue
      journal.value = journal.value.map(j => (s.acked.has(j.nonce) ? { ...j, synced: true } : j))
      for (const rej of s.rejected) {
        const op = ops.find(o => o.nonce === rej.nonce)
        rejections.value = [...rejections.value, { nonce: rej.nonce, what: describeOp(op), error: rej.error ?? 'rejected', at: nowIso() }]
        journal.value = journal.value.filter(j => j.nonce !== rej.nonce)
      }
      const b = bundle.value!
      bundle.value = { ...b, checkins: mergeCheckins(b.checkins, r.checkins ?? []), counters: r.counters ?? b.counters, cursor: r.cursor ?? b.cursor }
      lastSyncAt.value = nowIso()
      offline.value = false
      syncError.value = null
      await persistBundle()
      await persistQueue()
      if (queue.value.length) again = true
    } catch (e) {
      const err = toApiError(e)
      if (err.status === 401 || err.status === 403) {
        sessionEnded.value = true
        syncError.value = err.error
      } else if (!err.status || err.error === 'network_error') {
        offline.value = true
      } else if (err.status === 422 && bundle.value?.cursor) {
        // Unknown cursor (e.g. the server was restored): pull everything again; merging by nonce is idempotent.
        bundle.value = { ...bundle.value, cursor: '' }
        again = true
      } else {
        syncError.value = err.error
      }
    } finally {
      inflight.clear()
      syncing.value = false
    }
    if (again && !offline.value && !sessionEnded.value) {
      again = false
      await sync()
    }
  }

  async function pushAdds() {
    const batch = adds.value.slice(0, MAX_ADDS)
    let r: AddsResponse
    try {
      r = await apiFetch<AddsResponse>('/api/v1/door/adds', { method: 'POST', body: { adds: batch } })
    } catch (e) {
      // The event's guest data was erased (P2.5): these adds can never apply.
      if (toApiError(e).error !== 'event_purged') throw e
      r = { results: batch.map(a => ({ nonce: a.nonce, id: a.id, status: 'rejected' as const, error: 'event_purged' })) }
    }
    const answered = new Map(r.results.map(x => [x.nonce, x]))
    adds.value = adds.value.filter(a => !answered.has(a.nonce))
    for (const res of r.results) {
      if (res.status !== 'rejected') continue
      const a = batch.find(x => x.nonce === res.nonce)
      rejections.value = [...rejections.value, { nonce: res.nonce, what: `Add ${a?.name ?? ''}`.trim(), error: res.error ?? 'rejected', at: nowIso() }]
    }
  }

  function describeOp(op: DoorOp | undefined): string {
    if (!op) return 'Door action'
    if (op.type === 'counter') return op.kind === 'walkup' ? 'Walk-up' : op.kind === 'out' ? 'Out' : 'In'
    if (op.type === 'undo') return 'Undo'
    const b = bundle.value
    const name = op.subject.kind === 'guest'
      ? b?.guests.find(g => g.id === op.subject.id)?.name
      : b?.tickets.find(t => t.id === op.subject.id)?.name
    return `Check-in ${name ?? ''}`.trim()
  }

  function onOnline() {
    offline.value = false
    void sync()
  }
  function onOffline() {
    offline.value = true
  }
  async function tick() {
    if (await checkExpiry()) return
    await sync()
  }

  function startLoop() {
    if (timer || typeof window === 'undefined') return
    timer = setInterval(() => void tick(), SYNC_EVERY_MS)
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
  }

  function stopLoop() {
    if (timer) clearInterval(timer)
    timer = null
    if (typeof window !== 'undefined') {
      window.removeEventListener('online', onOnline)
      window.removeEventListener('offline', onOffline)
    }
  }

  // ---------------------------------------------------------------- staff side (event DOOR tab)

  async function call<T>(fn: () => Promise<T>): Promise<T> {
    try {
      return await fn()
    } catch (e) {
      throw toApiError(e)
    }
  }

  function loadDevice(): DoorDeviceRecord | null {
    device.value = readDevice()
    return device.value
  }

  /** Register this browser as a door device for an event; the token stays in localStorage. */
  async function registerDevice(label: string, event: DoorDeviceRecord['event']): Promise<DoorDeviceRecord> {
    const d = await call(() => apiFetch<RegisteredDevice>('/api/v1/door/devices', { method: 'POST', body: { label: label.trim() } }))
    const rec: DoorDeviceRecord = { id: d.id, label: d.label, token: d.token, event: { id: event.id, title: event.title, starts_at: event.starts_at } }
    writeDevice(rec)
    device.value = rec
    await fetchDevices().catch(() => undefined)
    return rec
  }

  /** Point this browser's device at another event (the next door login wipes the old cache). */
  function assignEvent(event: DoorDeviceRecord['event']) {
    if (!device.value) return
    device.value = { ...device.value, event: { id: event.id, title: event.title, starts_at: event.starts_at } }
    writeDevice(device.value)
  }

  /** Forget the device on this browser (token and cache). */
  async function forgetDevice() {
    writeDevice(null)
    device.value = null
    await wipe('logout')
    wipedBecause.value = null
  }

  async function fetchDevices(): Promise<void> {
    devices.value = await call(() => apiFetch<DoorDevice[]>('/api/v1/door/devices'))
  }

  async function revokeDevice(id: string): Promise<void> {
    await call(() => apiFetch(`/api/v1/door/devices/${id}`, { method: 'DELETE' }))
    if (device.value?.id === id) await forgetDevice()
    await fetchDevices()
  }

  async function setPin(eventId: string, input: PinInput): Promise<PinResult> {
    const r = await call(() => apiFetch<PinResult>(`/api/v1/door/events/${eventId}/pin`, { method: 'POST', body: input }))
    await fetchPinStatus(eventId).catch(() => undefined)
    return r
  }

  async function fetchPinStatus(eventId: string): Promise<void> {
    pinStatus.value = await call(() => apiFetch<PinStatus>(`/api/v1/door/events/${eventId}/pin`))
  }

  return {
    device, phase, bundle, queue, adds, journal, rejections, syncing, lastSyncAt, offline, syncError, sessionEnded, wipedBecause,
    devices, pinStatus, checkins, counters, occ, queued, recent, index,
    init, login, downloadBundle, wipe, logout, checkExpiry, checkIn, counter, undo, addGuest, dismissRejections, sync, startLoop, stopLoop,
    loadDevice, registerDevice, assignEvent, forgetDevice, fetchDevices, revokeDevice, setPin, fetchPinStatus,
  }
})
