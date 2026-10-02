// The receipt queue behind the entry form: client-side pre-checks, files
// waiting for an entry that does not exist yet, sequential upload, partial
// failure, retry and removal.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { useReceiptQueue } from '../useReceiptQueue'
import { useEarningsStore } from '../../stores/earnings'
import { deferred, httpError, makeAttachment, makeEntry, makeFile } from '../../components/finance/__tests__/fixtures'

const fetchMock = vi.fn()
let created: string[]
let revoked: string[]

beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
  created = []
  revoked = []
  // jsdom has no object URLs.
  vi.stubGlobal('URL', Object.assign(URL, {
    createObjectURL: (f: File) => { const u = `blob:${f.name}`; created.push(u); return u },
    revokeObjectURL: (u: string) => { revoked.push(u) },
  }))
})
afterEach(() => { vi.unstubAllGlobals() })

const png = (name = 'a.png', size = 1024) => makeFile(name, 'image/png', size)

describe('pre-checks (before any request)', () => {
  it('rejects by type, by size and when empty, with a message naming the file', () => {
    const q = useReceiptQueue()
    q.addFiles([
      makeFile('iphone.heic', 'image/heic'),
      makeFile('logo.svg', 'image/svg+xml'),
      makeFile('page.html', 'text/html'),
      makeFile('huge.pdf', 'application/pdf', 15 * 1024 * 1024 + 1),
      makeFile('empty.png', 'image/png', 0),
    ])
    expect(q.items).toHaveLength(0)
    expect(q.rejections).toEqual([
      'iphone.heic: use a JPEG, PNG, WebP or PDF file.',
      'logo.svg: use a JPEG, PNG, WebP or PDF file.',
      'page.html: use a JPEG, PNG, WebP or PDF file.',
      'huge.pdf is 15.1 MB. Receipts can be up to 15 MB.',
      'empty.png is empty.',
    ])
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('accepts the four types, and a file whose type is unknown but whose extension is fine', () => {
    const q = useReceiptQueue()
    q.addFiles([
      makeFile('a.jpg', 'image/jpeg'), makeFile('b.png', 'image/png'), makeFile('c.webp', 'image/webp'),
      makeFile('d.pdf', 'application/pdf'), makeFile('e.PDF', ''), makeFile('f.jpeg', 'application/octet-stream'),
    ])
    expect(q.items).toHaveLength(6)
    expect(q.rejections).toEqual([])
  })

  it('does not trust the extension over a wrong declared type', () => {
    const q = useReceiptQueue()
    q.addFiles([makeFile('renamed.pdf', 'text/html')])
    expect(q.items).toHaveLength(0)
    expect(q.rejections).toHaveLength(1)
  })

  it('holds at most 10 files in total and says how many were left out', () => {
    const q = useReceiptQueue()
    q.addFiles(Array.from({ length: 8 }, (_, i) => png(`${i}.png`)))
    q.addFiles(Array.from({ length: 4 }, (_, i) => png(`more-${i}.png`)))
    expect(q.items).toHaveLength(10)
    expect(q.rejections).toEqual(['An entry holds up to 10 receipts: 2 files not added.'])
  })

  it('clears the previous rejections on the next pick', () => {
    const q = useReceiptQueue()
    q.addFiles([makeFile('x.heic', 'image/heic')])
    expect(q.rejections).toHaveLength(1)
    q.addFiles([png()])
    expect(q.rejections).toEqual([])
  })
})

describe('before the entry exists', () => {
  it('queues files without any request and previews images with an object URL', () => {
    const q = useReceiptQueue()
    q.addFiles([png('photo.png'), makeFile('bill.pdf', 'application/pdf')])
    expect(fetchMock).not.toHaveBeenCalled()
    expect(q.items.map((i) => [i.name, i.status])).toEqual([['photo.png', 'queued'], ['bill.pdf', 'queued']])
    expect(q.items[0]!.preview).toBe('blob:photo.png')
    expect(q.items[1]!.preview).toBeNull() // a PDF has no thumbnail
    expect(q.queuedCount).toBe(2)
    expect(q.announcement).toMatch(/upload when the entry is saved/i)
  })

  it('uploads sequentially, in the order picked, once the entry exists', async () => {
    const q = useReceiptQueue()
    q.addFiles([png('1.png'), png('2.png'), png('3.png')])
    const gates = [deferred<unknown>(), deferred<unknown>(), deferred<unknown>()]
    let call = 0
    fetchMock.mockImplementation(() => gates[call++]!.promise)

    const done = q.uploadQueued('e-1')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(1) // the second waits for the first
    expect(q.items.map((i) => i.status)).toEqual(['uploading', 'queued', 'queued'])

    gates[0]!.resolve({ data: makeAttachment({ id: 'a-1', filename: '1.png' }) })
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(q.items.map((i) => i.status)).toEqual(['done', 'uploading', 'queued'])

    gates[1]!.resolve({ data: makeAttachment({ id: 'a-2', filename: '2.png' }) })
    await flushPromises()
    gates[2]!.resolve({ data: makeAttachment({ id: 'a-3', filename: '3.png' }) })
    expect(await done).toEqual({ failed: 0, total: 3 })

    const names = fetchMock.mock.calls.map(([, o]) => ((o.body as FormData).get('file') as File).name)
    expect(names).toEqual(['1.png', '2.png', '3.png'])
    expect(q.items.every((i) => i.status === 'done' && i.attachment && !i.file)).toBe(true)
    // The object URLs are given back once the files are stored.
    expect(revoked.sort()).toEqual(['blob:1.png', 'blob:2.png', 'blob:3.png'])
  })

  it('keeps a failed file with its server message and goes on with the next one', async () => {
    const q = useReceiptQueue()
    q.addFiles([png('ok.png'), makeFile('bad.pdf', 'application/pdf'), png('ok2.png')])
    fetchMock
      .mockResolvedValueOnce({ data: makeAttachment({ id: 'a-1', filename: 'ok.png' }) })
      .mockRejectedValueOnce(httpError(415, 'unsupported_media_type', 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file'))
      .mockResolvedValueOnce({ data: makeAttachment({ id: 'a-3', filename: 'ok2.png' }) })

    const result = await q.uploadQueued('e-1')

    expect(result).toEqual({ failed: 1, total: 3 })
    expect(q.items.map((i) => i.status)).toEqual(['done', 'failed', 'done'])
    expect(q.items[1]).toMatchObject({ error: 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file', name: 'bad.pdf' })
    expect(q.items[1]!.file).not.toBeNull() // still there to retry
  })

  it('announces a failed upload through the live region', async () => {
    const q = useReceiptQueue()
    q.addFiles([makeFile('bad.pdf', 'application/pdf')])
    fetchMock.mockRejectedValueOnce(httpError(415, 'unsupported_media_type', 'unsupported attachment type'))
    await q.uploadQueued('e-1')
    expect(q.announcement).toBe('bad.pdf did not upload. unsupported attachment type')
  })

  it('retries a failed file and it ends up saved', async () => {
    const q = useReceiptQueue()
    q.addFiles([png('flaky.png')])
    fetchMock.mockRejectedValueOnce(httpError(409, 'limit_reached', 'an entry can hold at most 10 files'))
    await q.uploadQueued('e-1')
    expect(q.items[0]!.status).toBe('failed')

    fetchMock.mockResolvedValueOnce({ data: makeAttachment({ id: 'a-1', filename: 'flaky.png' }) })
    q.retry(q.items[0]!.key)
    await flushPromises()
    expect(q.items[0]).toMatchObject({ status: 'done', error: '' })
    expect(q.failedCount).toBe(0)
  })

  it('drops a queued or failed file at once, without a confirmation, and frees its preview', () => {
    const q = useReceiptQueue()
    q.addFiles([png('gone.png'), png('stay.png')])
    q.remove(q.items[0]!.key)
    expect(q.items.map((i) => i.name)).toEqual(['stay.png'])
    expect(revoked).toEqual(['blob:gone.png'])
  })
})

describe('with an existing entry', () => {
  it('uploads a picked file immediately', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry()]
    const q = useReceiptQueue()
    q.bind('e-1')
    fetchMock.mockResolvedValueOnce({ data: makeAttachment({ filename: 'now.png' }) })
    q.addFiles([png('now.png')])
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(q.items[0]!.status).toBe('done')
    expect(store.entries[0]!.attachment_count).toBe(1)
  })

  it('loads the stored receipts and keeps files that are still waiting', async () => {
    const q = useReceiptQueue()
    q.addFiles([png('waiting.png')])
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment({ id: 'a-1', filename: 'saved.pdf', mime_type: 'application/pdf' })] })
    await q.loadSaved('e-1')
    expect(q.items.map((i) => [i.name, i.status])).toEqual([['saved.pdf', 'done'], ['waiting.png', 'queued']])
  })

  it('reports a receipts list that could not be loaded', async () => {
    const q = useReceiptQueue()
    fetchMock.mockRejectedValueOnce(httpError(404, 'not_found', 'finance entry not found'))
    await q.loadSaved('e-1')
    expect(q.loadError).toBe('finance entry not found')
  })

  it('asks before removing a stored receipt; KEEP cancels, YES deletes', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ attachment_count: 1 })]
    const q = useReceiptQueue()
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment()] })
    await q.loadSaved('e-1')

    q.remove(q.items[0]!.key)
    expect(q.items[0]!.confirming).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1) // only the list so far

    q.cancelRemove(q.items[0]!.key)
    expect(q.items[0]!.confirming).toBe(false)
    expect(q.items).toHaveLength(1)

    fetchMock.mockResolvedValueOnce(undefined)
    await q.confirmRemove(q.items[0]!.key)
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/finance/entries/e-1/attachments/a-1', { method: 'DELETE' })
    expect(q.items).toHaveLength(0)
    expect(store.entries[0]!.attachment_count).toBe(0)
  })

  it('keeps the receipt and shows the reason when the server refuses to remove it', async () => {
    const q = useReceiptQueue()
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment()] })
    await q.loadSaved('e-1')
    q.remove(q.items[0]!.key)
    fetchMock.mockRejectedValueOnce(httpError(409, 'inactive', 'voided finance entries cannot change their attachments'))
    await q.confirmRemove(q.items[0]!.key)
    expect(q.items[0]).toMatchObject({ status: 'done', error: 'voided finance entries cannot change their attachments' })
  })

  it('stops uploading and frees previews when disposed', async () => {
    const q = useReceiptQueue()
    q.addFiles([png('1.png'), png('2.png')])
    const gate = deferred<unknown>()
    fetchMock.mockReturnValue(gate.promise)
    void q.uploadQueued('e-1')
    await flushPromises()
    q.dispose()
    gate.resolve({ data: makeAttachment() })
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(1) // the second file never starts
    expect(revoked).toContain('blob:2.png')
  })
})
