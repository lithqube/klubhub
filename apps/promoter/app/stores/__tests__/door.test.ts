import { pbkdf2Sync } from 'node:crypto'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { DoorBundle, DoorDeviceRecord } from '~/types/door'
import { memoryKV, type KV } from '~/utils/doorDb'
import { DEVICE_KEY, setDoorKV, useDoorStore } from '../door'

const fetchMock = vi.fn()
vi.stubGlobal('$fetch', fetchMock)

// Node 22+ ships its own (file-less, unusable) localStorage that shadows jsdom's.
const mem = new Map<string, string>()
vi.stubGlobal('localStorage', {
  getItem: (k: string) => mem.get(k) ?? null,
  setItem: (k: string, v: string) => void mem.set(k, String(v)),
  removeItem: (k: string) => void mem.delete(k),
  clear: () => mem.clear(),
})

const salt = Buffer.from('0123456789abcdef')
const managerPin = { salt: salt.toString('base64'), iterations: 1000, hash: pbkdf2Sync('246810', salt, 1000, 32, 'sha256').toString('base64') }

const device: DoorDeviceRecord = { id: 'd-1', label: 'Front door', token: 'tok', event: { id: 'e1', title: 'Klubnacht', starts_at: '2026-10-03T21:00:00Z' } }
const later = (h: number) => new Date(Date.now() + h * 3_600_000).toISOString()
const bundle = (): DoorBundle => ({
  generated_at: new Date().toISOString(), device_id: 'd-1', session_expires_at: later(10),
  event: { id: 'e1', title: 'Klubnacht', starts_at: later(-1), ends_at: later(6), doors_at: null, timezone: 'Europe/Berlin', capacity: 100 },
  lists: [{ id: 'l1', name: 'Comp', type: 'comp', entry_terms: { price_mode: 'free', reduced_price_text: '', cutoff_at: null, perks: [] } }],
  guests: [{ id: 'g1', list_id: 'l1', name: 'Mara Weiss', plus_n: 2, status: 'going', note: '' }],
  tickets: [{ id: 't1', name: 'Hana Kim', ticket_type: 'Early', order_ref: 'D-1', source: 'dice', status: 'valid', secret: 'S1' }],
  checkins: [], counters: { walkups: 0, manual_in: 0, manual_out: 0 }, cursor: 'c0', manager_pin: managerPin,
})

const syncOk = (body: { ops: { nonce: string }[] }) => ({
  results: body.ops.map(o => ({ nonce: o.nonce, status: 'applied' })), checkins: [], counters: { walkups: 0, manual_in: 0, manual_out: 0 }, cursor: 'c1',
})

let kv: KV

async function ready() {
  localStorage.setItem(DEVICE_KEY, JSON.stringify(device))
  const s = useDoorStore()
  await s.init()
  fetchMock.mockImplementation((url: string) => (url === '/api/v1/door/bundle' ? Promise.resolve(bundle()) : Promise.resolve(null)))
  await s.login('123456')
  return s
}

describe('useDoorStore (door device)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    localStorage.clear()
    kv = memoryKV()
    setDoorKV(() => kv)
    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
  })
  afterEach(() => useDoorStore().stopLoop())

  it('is unprepared without a device record and locked with one', async () => {
    const s = useDoorStore()
    await s.init()
    expect(s.phase).toBe('unprepared')
    localStorage.setItem(DEVICE_KEY, JSON.stringify(device))
    await s.init()
    expect(s.phase).toBe('locked')
    expect(s.device?.label).toBe('Front door')
  })

  it('logs in with the device token and PIN, then keeps the bundle encrypted', async () => {
    const s = await ready()
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/door/login')
    expect(opts.body).toEqual({ device_token: 'tok', event_id: 'e1', pin: '123456' })
    expect(opts.headers['X-KlubHub-CSRF']).toBe('1')
    expect(s.phase).toBe('ready')
    const raw = await kv.get('slot:bundle') as { ct: ArrayBuffer }
    expect(new TextDecoder().decode(new Uint8Array(raw.ct))).not.toContain('Mara')

    // A reload reopens the session from the vault without the network.
    setActivePinia(createPinia())
    fetchMock.mockReset()
    const again = useDoorStore()
    await again.init()
    expect(again.phase).toBe('ready')
    expect(again.bundle?.guests[0]!.name).toBe('Mara Weiss')
    expect(fetchMock).not.toHaveBeenCalled()
    again.stopLoop()
  })

  it('reports a wrong PIN and stays locked', async () => {
    localStorage.setItem(DEVICE_KEY, JSON.stringify(device))
    const s = useDoorStore()
    await s.init()
    fetchMock.mockRejectedValue({ statusCode: 401, data: { error: 'invalid_credentials' } })
    await expect(s.login('000000')).rejects.toMatchObject({ error: 'invalid_credentials', status: 401 })
    expect(s.phase).toBe('locked')
  })

  it('queues check-ins offline with nonces and syncs them in order when back online', async () => {
    const s = await ready()
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    fetchMock.mockReset()
    const n1 = await s.checkIn({ kind: 'guest', id: 'g1' }, 2)
    await s.counter('walkup')
    expect(s.offline).toBe(true)
    expect(s.queued).toBe(2)
    expect(s.occ.inside).toBe(3)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(n1).toMatch(/^[0-9a-f-]{36}$/)

    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
    fetchMock.mockImplementation((_url: string, opts: { body: { ops: { nonce: string }[] } }) => Promise.resolve({
      ...syncOk(opts.body),
      checkins: [{ nonce: n1, subject: { kind: 'guest', id: 'g1' }, count: 2, direction: 'in', at: new Date().toISOString(), device_id: 'd-1', undone: false, conflict: false }],
      counters: { walkups: 1, manual_in: 0, manual_out: 0 },
    }))
    await s.sync()
    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/door/checkins')
    expect(opts.body.since).toBe('c0')
    expect(opts.body.ops.map((o: { type: string }) => o.type)).toEqual(['checkin', 'counter'])
    expect(s.queued).toBe(0)
    expect(s.offline).toBe(false)
    expect(s.occ.inside).toBe(3) // server now holds what the queue held
    expect(s.bundle?.cursor).toBe('c1')
    expect(s.lastSyncAt).not.toBeNull()
  })

  it('undo of a queued check-in never reaches the server; undo of a synced one is sent', async () => {
    const s = await ready()
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    const n = await s.checkIn({ kind: 'ticket', id: 't1' }, 1)
    await s.undo(n)
    expect(s.queue).toHaveLength(0)
    expect(s.occ.inside).toBe(0)

    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
    fetchMock.mockReset()
    fetchMock.mockImplementation((_u: string, o: { body: { ops: { nonce: string }[] } }) => Promise.resolve(syncOk(o.body)))
    const m = await s.checkIn({ kind: 'ticket', id: 't1' }, 1)
    await vi.waitFor(() => expect(s.queue).toHaveLength(0))
    await s.undo(m)
    await vi.waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2))
    const last = fetchMock.mock.calls.at(-1)![1].body.ops
    expect(last).toEqual([expect.objectContaining({ type: 'undo', target: m })])
  })

  it('records rejected ops and ends the session on 401 without losing the queue', async () => {
    const s = await ready()
    fetchMock.mockReset()
    fetchMock.mockImplementation((_u: string, o: { body: { ops: { nonce: string }[] } }) => Promise.resolve({
      ...syncOk(o.body), results: o.body.ops.map(x => ({ nonce: x.nonce, status: 'rejected', error: 'unknown_subject' })),
    }))
    await s.checkIn({ kind: 'guest', id: 'g1' }, 1)
    await vi.waitFor(() => expect(s.rejections).toHaveLength(1))
    expect(s.rejections[0]).toMatchObject({ what: 'Check-in Mara Weiss', error: 'unknown_subject' })

    fetchMock.mockReset()
    fetchMock.mockRejectedValue({ statusCode: 401, data: { error: 'unauthenticated' } })
    await s.counter('out')
    await vi.waitFor(() => expect(s.sessionEnded).toBe(true))
    expect(s.queued).toBe(1)
  })

  it('adds a guest after an offline manager-PIN check, and syncs the add before its check-in', async () => {
    const s = await ready()
    await expect(s.addGuest({ name: 'Ola', plus_n: 0, list_id: 'l1', manager_pin: '111111', count: 1 }))
      .rejects.toMatchObject({ error: 'manager_pin_invalid' })
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    const { id } = await s.addGuest({ name: '  Ola  Nordmann ', plus_n: 1, list_id: 'l1', manager_pin: '246810', count: 2 })
    expect(s.bundle?.guests.find(g => g.id === id)?.name).toBe('Ola Nordmann')
    expect(s.queued).toBe(2)
    expect(s.occ.inside).toBe(2)

    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
    fetchMock.mockReset()
    fetchMock.mockImplementation((url: string, o: { body: { adds?: { nonce: string, id: string }[], ops?: { nonce: string }[] } }) =>
      Promise.resolve(url.endsWith('/adds')
        ? { results: o.body.adds!.map(a => ({ nonce: a.nonce, id: a.id, status: 'applied' })) }
        : syncOk(o.body as { ops: { nonce: string }[] })))
    await s.sync()
    expect(fetchMock.mock.calls.map(c => c[0])).toEqual(['/api/v1/door/adds', '/api/v1/door/checkins'])
    expect(fetchMock.mock.calls[0]![1].body.adds[0]).toMatchObject({ id, name: 'Ola Nordmann', plus_n: 1, manager_pin: '246810', list_id: 'l1' })
    expect(s.queued).toBe(0)
  })

  it('drops door adds refused because the event was erased, and says why', async () => {
    const s = await ready()
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    await s.addGuest({ name: 'Ola Nordmann', plus_n: 0, list_id: 'l1', manager_pin: '246810', count: 1 })
    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
    fetchMock.mockReset()
    fetchMock.mockImplementation((url: string, o: { body: { ops?: { nonce: string }[] } }) => (url.endsWith('/adds')
      ? Promise.reject({ statusCode: 409, data: { error: 'event_purged' } })
      : Promise.resolve(syncOk(o.body as { ops: { nonce: string }[] }))))
    await s.sync()
    expect(s.adds).toEqual([])
    expect(s.rejections).toEqual([expect.objectContaining({ what: 'Add Ola Nordmann', error: 'event_purged' })])
  })

  it('keeps unsynced actions when the door session expires, and asks for a new PIN instead of wiping', async () => {
    const s = await ready()
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    await s.checkIn({ kind: 'guest', id: 'g1' }, 1)
    expect(await s.checkExpiry(Date.now() + 11 * 3_600_000)).toBe(true)
    expect(s.sessionEnded).toBe(true)
    expect(s.phase).toBe('ready')
    expect(s.queued).toBe(1)
    expect(await kv.get('slot:bundle')).toBeDefined()

    // A reload after the window keeps the queue too.
    s.stopLoop()
    const realNow = Date.now
    vi.spyOn(Date, 'now').mockReturnValue(realNow() + 11 * 3_600_000)
    setActivePinia(createPinia())
    const again = useDoorStore()
    await again.init()
    expect(again.phase).toBe('ready')
    expect(again.sessionEnded).toBe(true)
    expect(again.queued).toBe(1)
    vi.restoreAllMocks()

    // Logging in again carries the queue into the new session.
    Object.defineProperty(navigator, 'onLine', { value: true, configurable: true })
    fetchMock.mockReset()
    fetchMock.mockImplementation((url: string, o?: { body?: { ops: { nonce: string }[] } }) => Promise.resolve(
      url === '/api/v1/door/bundle' ? bundle() : url === '/api/v1/door/checkins' ? syncOk(o!.body!) : null))
    await again.login('654321')
    expect(again.sessionEnded).toBe(false)
    await vi.waitFor(() => expect(again.queued).toBe(0))
    expect(fetchMock.mock.calls.some(c => c[0] === '/api/v1/door/checkins')).toBe(true)
    again.stopLoop()
  })

  it('passes a PIN lockout on with its retry time', async () => {
    localStorage.setItem(DEVICE_KEY, JSON.stringify(device))
    const s = useDoorStore()
    await s.init()
    fetchMock.mockRejectedValue({ statusCode: 429, data: { error: 'pin_locked', retry_after: '2026-10-03T23:47:00Z' } })
    await expect(s.login('111111')).rejects.toMatchObject({ error: 'pin_locked', status: 429, detail: { retry_after: '2026-10-03T23:47:00Z' } })
    fetchMock.mockRejectedValue({ statusCode: 401, data: { error: 'pin_expired' } })
    await expect(s.login('999999')).rejects.toMatchObject({ error: 'pin_expired', status: 401 })
    expect(s.phase).toBe('locked')
  })

  it('offers own actions in RECENT until they are undone', async () => {
    const s = await ready()
    Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
    const a = await s.checkIn({ kind: 'guest', id: 'g1' }, 2)
    const b = await s.counter('walkup')
    expect(s.recent.map(r => r.nonce)).toEqual([b, a])
    await s.undo(a)
    expect(s.recent.map(r => r.nonce)).toEqual([b])
  })

  it('wipes the cache on logout and when the session expires', async () => {
    const s = await ready()
    fetchMock.mockResolvedValue(null)
    await s.logout()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/auth/logout', expect.objectContaining({ method: 'POST' }))
    expect(s.phase).toBe('locked')
    expect(s.bundle).toBeNull()
    expect(await kv.get('slot:bundle')).toBeUndefined()
    expect(s.wipedBecause).toBe('logout')

    const t = await ready()
    expect(await t.checkExpiry(Date.now() + 11 * 3_600_000)).toBe(true)
    expect(t.wipedBecause).toBe('session')
    expect(await kv.get('session-key')).toBeUndefined()
  })
})

describe('useDoorStore (event DOOR tab)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    localStorage.clear()
    setDoorKV(() => memoryKV())
  })

  it('registers this browser and keeps the token with the event in localStorage', async () => {
    fetchMock.mockImplementation((url: string, opts?: { method?: string }) =>
      Promise.resolve(opts?.method === 'POST' ? { id: 'd-9', label: 'Front', token: 'secret-token' } : []))
    const s = useDoorStore()
    await s.registerDevice(' Front ', { id: 'e1', title: 'Klubnacht', starts_at: 'x' })
    expect(fetchMock.mock.calls[0]![1].body).toEqual({ label: 'Front' })
    expect(JSON.parse(localStorage.getItem(DEVICE_KEY)!)).toEqual({ id: 'd-9', label: 'Front', token: 'secret-token', event: { id: 'e1', title: 'Klubnacht', starts_at: 'x' } })
    s.assignEvent({ id: 'e2', title: 'Warehouse', starts_at: 'y' })
    expect(JSON.parse(localStorage.getItem(DEVICE_KEY)!).event.id).toBe('e2')
  })

  it('revoking this browser\'s own device forgets it locally', async () => {
    localStorage.setItem(DEVICE_KEY, JSON.stringify(device))
    fetchMock.mockResolvedValue([])
    const s = useDoorStore()
    s.loadDevice()
    await s.revokeDevice('d-1')
    expect(fetchMock.mock.calls[0]).toEqual(['/api/v1/door/devices/d-1', expect.objectContaining({ method: 'DELETE' })])
    expect(localStorage.getItem(DEVICE_KEY)).toBeNull()
    expect(s.device).toBeNull()
  })

  it('sets a manager PIN and refreshes the PIN status', async () => {
    fetchMock.mockImplementation((_url: string, opts?: { method?: string }) => Promise.resolve(opts?.method === 'POST'
      ? { pin: '246810', valid_until: '2026-10-04T09:00:00Z', manager: true }
      : { staff: null, manager: { valid_until: '2026-10-04T09:00:00Z' } }))
    const s = useDoorStore()
    const r = await s.setPin('e1', { manager: true, valid_until: '2026-10-04T09:00:00Z' })
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/door/events/e1/pin')
    expect(fetchMock.mock.calls[0]![1].body).toEqual({ manager: true, valid_until: '2026-10-04T09:00:00Z' })
    expect(r.pin).toBe('246810')
    expect(s.pinStatus?.manager?.valid_until).toBe('2026-10-04T09:00:00Z')
  })
})
