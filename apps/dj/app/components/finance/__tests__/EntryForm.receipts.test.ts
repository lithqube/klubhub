// EntryForm receipts: the RECEIPTS section of the inline entry composer.
// Picking (camera button, file button, drag and drop), client-side pre-checks,
// the create-then-upload flow (including partial failure), the edit flow, and
// the read-only view of a voided entry.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import EntryForm from '../EntryForm.vue'
import { useEarningsStore } from '../../../stores/earnings'
import type { Entry } from '../../../types/finance'
import { deferred, httpError, makeAttachment, makeEntry, makeFile } from './fixtures'

const fetchMock = vi.fn()
const wrappers: VueWrapper[] = []

type Call = { url: string; method: string; body: unknown }
const calls = (): Call[] => fetchMock.mock.calls.map(([url, o]) => ({ url, method: o?.method ?? 'GET', body: o?.body }))
const uploads = () => calls().filter((c) => c.method === 'POST' && c.url.endsWith('/attachments'))

function mountForm(props: { kind?: Entry['kind']; editId?: string | null } = {}) {
  const w = mount(EntryForm, { attachTo: document.body, props: { kind: 'expense', editId: null, ...props } })
  wrappers.push(w)
  return w
}

async function setField(selector: string, value: string): Promise<void> {
  const el = document.querySelector(selector) as HTMLInputElement
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  el.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

async function fillExpense(): Promise<void> {
  await setField('#ee-amount', '50')
  await setField('#ee-notes', 'New cable')
}

async function pick(testid: string, files: File[]): Promise<void> {
  const input = document.querySelector(`[data-testid="${testid}"]`) as HTMLInputElement
  Object.defineProperty(input, 'files', { configurable: true, value: files })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

async function drop(files: File[]): Promise<void> {
  const zone = document.querySelector('[data-testid="rc-dropzone"]') as HTMLElement
  const ev = new Event('drop', { bubbles: true, cancelable: true })
  Object.defineProperty(ev, 'dataTransfer', { value: { files, items: [], types: ['Files'] } })
  zone.dispatchEvent(ev)
  await flushPromises()
}

async function submit(): Promise<void> {
  document.querySelector('form.ee-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await flushPromises()
}

const button = (label: string) =>
  Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === label) as HTMLButtonElement | undefined
const rows = () => Array.from(document.querySelectorAll('.rc-item')) as HTMLElement[]
const rowText = (i: number) => rows()[i]!.textContent!.replace(/\s+/g, ' ').trim()
const live = () => document.querySelector('.rc [role="status"][aria-live="polite"]')!.textContent

beforeEach(() => {
  setActivePinia(createPinia())
  document.body.innerHTML = ''
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
  vi.stubGlobal('URL', Object.assign(URL, {
    createObjectURL: (f: File) => `blob:${f.name}`,
    revokeObjectURL: () => undefined,
  }))
})
afterEach(() => {
  wrappers.splice(0).forEach((w) => w.unmount())
  vi.unstubAllGlobals()
})

describe('RECEIPTS section', () => {
  it('has a RECEIPTS label, the format hint, and no request until something is picked', async () => {
    mountForm()
    await flushPromises()
    expect(document.querySelector('#rc-title')!.textContent).toBe('RECEIPTS')
    expect(document.body.textContent).toContain('JPEG, PNG, WebP or PDF, up to 15 MB each, 10 per entry')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('TAKE PHOTO opens the rear camera on a phone (capture) and accepts any image', async () => {
    mountForm()
    const input = document.querySelector('[data-testid="rc-photo-input"]') as HTMLInputElement
    expect(input.getAttribute('type')).toBe('file')
    expect(input.getAttribute('accept')).toBe('image/*')
    expect(input.getAttribute('capture')).toBe('environment')
    expect(input.multiple).toBe(false)
    const click = vi.spyOn(input, 'click')
    button('TAKE PHOTO')!.click()
    expect(click).toHaveBeenCalledTimes(1)
  })

  it('ADD FILE accepts JPEG, PNG, WebP and PDF, several at a time, with no camera capture', async () => {
    mountForm()
    const input = document.querySelector('[data-testid="rc-file-input"]') as HTMLInputElement
    expect(input.getAttribute('accept')).toBe('image/jpeg,image/png,image/webp,application/pdf')
    expect(input.multiple).toBe(true)
    expect(input.hasAttribute('capture')).toBe(false)
    const click = vi.spyOn(input, 'click')
    button('ADD FILE')!.click()
    expect(click).toHaveBeenCalledTimes(1)
  })

  it('lists picked files as queued, with a thumbnail for images and a glyph for PDFs', async () => {
    mountForm()
    await pick('rc-file-input', [makeFile('cable.png', 'image/png', 2048), makeFile('invoice.pdf', 'application/pdf', 340 * 1024)])
    expect(rows()).toHaveLength(2)
    expect(rowText(0)).toContain('cable.png')
    expect(rowText(0)).toContain('2 KB')
    expect(rowText(0)).toContain('QUEUED')
    expect(rows()[0]!.querySelector('img')!.getAttribute('src')).toBe('blob:cable.png')
    expect(rows()[1]!.querySelector('img')).toBeNull()
    expect(rowText(1)).toContain('340 KB')
    expect(document.querySelector('ul[aria-label="Receipts"]')).not.toBeNull()
    expect(live()).toMatch(/2 receipts added/)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('takes files dropped on the drop zone', async () => {
    mountForm()
    await flushPromises() // the drop listener attaches once the zone is mounted
    await drop([makeFile('dropped.jpg', 'image/jpeg')])
    expect(rows()).toHaveLength(1)
    expect(rowText(0)).toContain('dropped.jpg')
  })

  it('refuses HEIC, SVG, oversize and surplus files with a message and no request', async () => {
    mountForm()
    await pick('rc-file-input', [
      makeFile('IMG_0001.HEIC', 'image/heic'),
      makeFile('logo.svg', 'image/svg+xml'),
      makeFile('scan.pdf', 'application/pdf', 20 * 1024 * 1024),
    ])
    expect(rows()).toHaveLength(0)
    const alert = document.querySelector('.rc [role="alert"]')!
    expect(alert.textContent).toContain('IMG_0001.HEIC: use a JPEG, PNG, WebP or PDF file.')
    expect(alert.textContent).toContain('logo.svg')
    expect(alert.textContent).toContain('scan.pdf is 20 MB. Receipts can be up to 15 MB.')
    expect(fetchMock).not.toHaveBeenCalled()

    await pick('rc-file-input', Array.from({ length: 11 }, (_, i) => makeFile(`r${i}.png`, 'image/png')))
    expect(rows()).toHaveLength(10)
    expect(document.querySelector('.rc [role="alert"]')!.textContent).toContain('1 file not added')
  })

  it('removes a queued file with a labelled button, without a confirmation', async () => {
    mountForm()
    await pick('rc-file-input', [makeFile('a.png', 'image/png'), makeFile('b.png', 'image/png')])
    const remove = document.querySelector('[aria-label="Remove receipt a.png"]') as HTMLButtonElement
    expect(remove).not.toBeNull()
    remove.click()
    await flushPromises()
    expect(rows().map((r) => r.textContent)).toHaveLength(1)
    expect(rowText(0)).toContain('b.png')
  })
})

describe('create, then upload', () => {
  it('creates the entry first, then uploads the queued files one after the other, then closes', async () => {
    const order: string[] = []
    const gates = [deferred<unknown>(), deferred<unknown>()]
    let n = 0
    fetchMock.mockImplementation(async (url: string, o: { method?: string }) => {
      if (url === '/api/v1/finance/entries') { order.push('create'); return { data: makeEntry({ id: 'new-1', updated_at: 'T1' }) } }
      if (o?.method === 'POST' && url.endsWith('/attachments')) { order.push(`upload-${n}`); return gates[n++]!.promise }
      throw new Error(`unexpected ${url}`)
    })
    const w = mountForm()
    await fillExpense()
    await pick('rc-file-input', [makeFile('1.png', 'image/png'), makeFile('2.pdf', 'application/pdf')])

    await submit()
    expect(order).toEqual(['create', 'upload-0']) // the second file waits for the first
    expect(rowText(0)).toContain('UPLOADING')
    expect(rowText(1)).toContain('QUEUED')
    expect(w.emitted('close')).toBeUndefined()

    gates[0]!.resolve({ data: makeAttachment({ id: 'a-1', entry_id: 'new-1', filename: '1.png' }) })
    await flushPromises()
    expect(order).toEqual(['create', 'upload-0', 'upload-1'])
    gates[1]!.resolve({ data: makeAttachment({ id: 'a-2', entry_id: 'new-1', filename: '2.pdf', mime_type: 'application/pdf' }) })
    await flushPromises()

    expect(w.emitted('saved')).toHaveLength(1)
    expect(w.emitted('close')).toHaveLength(1)
    expect(uploads().map((c) => c.url)).toEqual(Array(2).fill('/api/v1/finance/entries/new-1/attachments'))
  })

  it('keeps everything when the entry itself cannot be created, and uploads nothing', async () => {
    fetchMock.mockRejectedValue(httpError(400, 'validation_failed', 'validation failed: notes: is required for expense'))
    const w = mountForm()
    await fillExpense()
    await pick('rc-file-input', [makeFile('1.png', 'image/png'), makeFile('2.png', 'image/png')])
    await submit()

    expect(uploads()).toHaveLength(0)
    expect(w.emitted('close')).toBeUndefined()
    expect(w.emitted('saved')).toBeUndefined()
    expect(document.querySelector('.ee-form [role="alert"]')!.textContent).toContain('validation failed')
    expect(rows()).toHaveLength(2)
    expect(rowText(0)).toContain('QUEUED')
    expect((document.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('New cable')
  })

  it('leaves the saved entry open with the failed file and a live message when an upload fails', async () => {
    fetchMock.mockImplementation(async (url: string, o: { method?: string; body?: FormData }) => {
      if (url === '/api/v1/finance/entries') return { data: makeEntry({ id: 'new-1', updated_at: 'T1' }) }
      if (o?.method === 'POST') {
        const name = (o.body!.get('file') as File).name
        if (name === 'broken.pdf') throw httpError(415, 'unsupported_media_type', 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file')
        return { data: makeAttachment({ id: `a-${name}`, entry_id: 'new-1', filename: name }) }
      }
      throw new Error(`unexpected ${url}`)
    })
    const w = mountForm()
    await fillExpense()
    await pick('rc-file-input', [makeFile('a.png', 'image/png'), makeFile('broken.pdf', 'application/pdf'), makeFile('c.png', 'image/png')])
    await submit()

    // The entry is saved and reported as saved...
    expect(w.emitted('saved')).toHaveLength(1)
    // ...but the form is NOT closed, and it is now an edit of that entry.
    expect(w.emitted('close')).toBeUndefined()
    expect(document.querySelector('#ee-title')!.textContent).toBe('EDIT EXPENSE')
    const note = document.querySelector('.ee-note')!
    expect(note.textContent).toBe('Entry saved. 1 of 3 receipts failed to upload.')
    expect(note.getAttribute('aria-live')).toBe('polite')
    expect(button('CLOSE')).toBeDefined()
    expect(button('SAVE CHANGES')).toBeDefined()
    // The failed file is still there with the server's reason and a way to retry or drop it.
    expect(rows().map((r) => /SAVED|FAILED/.exec(r.textContent!)![0])).toEqual(['SAVED', 'FAILED', 'SAVED'])
    expect(rowText(1)).toContain('broken.pdf')
    expect(rowText(1)).toContain('use a JPEG, PNG, WebP or PDF file')
    expect(document.querySelector('[aria-label="Retry upload of broken.pdf"]')).not.toBeNull()
    expect(document.querySelector('[aria-label="Remove receipt broken.pdf"]')).not.toBeNull()
  })

  it('retries the failed file against the saved entry, and SAVE CHANGES then uses the fresh token', async () => {
    let failing = true
    fetchMock.mockImplementation(async (url: string, o: { method?: string; body?: FormData }) => {
      if (url === '/api/v1/finance/entries' && o?.method === 'POST') return { data: makeEntry({ id: 'new-1', updated_at: 'T1' }) }
      if (o?.method === 'POST') {
        if (failing) throw httpError(409, 'limit_reached', 'an entry can hold at most 10 files')
        return { data: makeAttachment({ entry_id: 'new-1', filename: 'a.png' }) }
      }
      if (o?.method === 'PUT') return { data: makeEntry({ id: 'new-1', updated_at: 'T2' }) }
      throw new Error(`unexpected ${url}`)
    })
    const w = mountForm()
    await fillExpense()
    await pick('rc-file-input', [makeFile('a.png', 'image/png')])
    await submit()
    expect(document.querySelector('.ee-note')!.textContent).toBe('Entry saved. 1 of 1 receipt failed to upload.')

    failing = false
    ;(document.querySelector('[aria-label="Retry upload of a.png"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(rowText(0)).toContain('SAVED')
    expect(document.querySelector('[aria-label="Retry upload of a.png"]')).toBeNull()
    expect(uploads()).toHaveLength(2)
    expect(uploads()[1]!.url).toBe('/api/v1/finance/entries/new-1/attachments')

    await submit()
    const put = calls().find((c) => c.method === 'PUT')!
    expect(put.url).toBe('/api/v1/finance/entries/new-1')
    expect((put.body as { updated_at: string }).updated_at).toBe('T1')
    expect(w.emitted('close')).toHaveLength(1) // nothing left to retry: it closes now
  })

  it('does not submit twice while a receipt is uploading', async () => {
    const gate = deferred<unknown>()
    fetchMock.mockImplementation(async (url: string, o: { method?: string }) => {
      if (url === '/api/v1/finance/entries') return { data: makeEntry({ id: 'new-1' }) }
      if (o?.method === 'POST') return gate.promise
      throw new Error(`unexpected ${url}`)
    })
    mountForm()
    await fillExpense()
    await pick('rc-file-input', [makeFile('a.png', 'image/png')])
    await submit()
    await submit()
    expect(calls().filter((c) => c.url === '/api/v1/finance/entries')).toHaveLength(1)
    expect(button('UPLOADING RECEIPTS…')!.disabled).toBe(true)
  })
})

describe('edit', () => {
  function seed(over: Partial<Entry> = {}) {
    const store = useEarningsStore()
    store.entries = [makeEntry({ attachment_count: 1, ...over })]
    return store
  }

  it('shows the stored receipts: thumbnail from the inline URL, link to the plain download URL', async () => {
    seed()
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment(), makeAttachment({ id: 'a-2', filename: 'train.pdf', mime_type: 'application/pdf' })] })
    mountForm({ editId: 'e-1' })
    await flushPromises()
    expect(rows()).toHaveLength(2)
    expect(rows()[0]!.querySelector('img')!.getAttribute('src')).toBe('/api/v1/finance/entries/e-1/attachments/a-1?inline=1')
    const link = rows()[0]!.querySelector('a')!
    expect(link.getAttribute('href')).toBe('/api/v1/finance/entries/e-1/attachments/a-1')
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toBe('noopener')
    expect(rows()[1]!.querySelector('img')).toBeNull() // a PDF gets a glyph, not an <img>
    expect(rowText(0)).toContain('SAVED')
  })

  it('does not ask for the list when the entry has no receipts', async () => {
    seed({ attachment_count: 0 })
    mountForm({ editId: 'e-1' })
    await flushPromises()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('uploads a picked file at once, without saving the form', async () => {
    seed({ attachment_count: 0 })
    fetchMock.mockResolvedValue({ data: makeAttachment({ id: 'new', filename: 'later.jpg', mime_type: 'image/jpeg' }) })
    const w = mountForm({ editId: 'e-1' })
    await flushPromises()
    await pick('rc-photo-input', [makeFile('later.jpg', 'image/jpeg')])
    expect(uploads()).toHaveLength(1)
    expect(uploads()[0]!.url).toBe('/api/v1/finance/entries/e-1/attachments')
    expect(rowText(0)).toContain('SAVED')
    expect(calls().some((c) => c.method === 'PUT')).toBe(false)
    expect(w.emitted('close')).toBeUndefined()
    expect(useEarningsStore().entries[0]!.attachment_count).toBe(1)
    expect(live()).toBe('later.jpg saved.')
  })

  it('shows a server refusal on that file and keeps the rest of the form intact', async () => {
    seed({ attachment_count: 0 })
    fetchMock.mockRejectedValue(httpError(415, 'unsupported_media_type', 'unsupported attachment type: use a JPEG, PNG, WebP or PDF file'))
    mountForm({ editId: 'e-1' })
    await flushPromises()
    await pick('rc-file-input', [makeFile('looks-fine.png', 'image/png')]) // really a HEIC inside
    expect(rowText(0)).toContain('FAILED')
    expect(rowText(0)).toContain('unsupported attachment type: use a JPEG, PNG, WebP or PDF file')
    expect((document.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('New cable')
  })

  it('removes a stored receipt only after an inline confirmation (no dialog)', async () => {
    seed()
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment()] })
    mountForm({ editId: 'e-1' })
    await flushPromises()

    ;(document.querySelector('[aria-label="Remove receipt receipt.png"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(rowText(0)).toContain('REMOVE RECEIPT?')
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(calls().some((c) => c.method === 'DELETE')).toBe(false)
    // Focus lands on the safe choice.
    expect(document.activeElement).toBe(button('KEEP'))

    button('KEEP')!.click()
    await flushPromises()
    expect(rowText(0)).not.toContain('REMOVE RECEIPT?')
    expect(calls().some((c) => c.method === 'DELETE')).toBe(false)

    fetchMock.mockResolvedValueOnce(undefined)
    ;(document.querySelector('[aria-label="Remove receipt receipt.png"]') as HTMLButtonElement).click()
    await flushPromises()
    button('YES, REMOVE')!.click()
    await flushPromises()
    expect(calls().filter((c) => c.method === 'DELETE').map((c) => c.url)).toEqual(['/api/v1/finance/entries/e-1/attachments/a-1'])
    expect(rows()).toHaveLength(0)
    expect(useEarningsStore().entries[0]!.attachment_count).toBe(0)
    expect(live()).toBe('receipt.png removed.')
  })

  it('Escape cancels an open removal question before it closes the form', async () => {
    seed()
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment()] })
    const w = mountForm({ editId: 'e-1' })
    await flushPromises()
    ;(document.querySelector('[aria-label="Remove receipt receipt.png"]') as HTMLButtonElement).click()
    await flushPromises()
    button('KEEP')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(rowText(0)).not.toContain('REMOVE RECEIPT?')
    expect(w.emitted('close')).toBeUndefined()
  })

  it('reports a failed removal on the receipt and keeps it', async () => {
    seed()
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment()] })
    mountForm({ editId: 'e-1' })
    await flushPromises()
    ;(document.querySelector('[aria-label="Remove receipt receipt.png"]') as HTMLButtonElement).click()
    await flushPromises()
    fetchMock.mockRejectedValueOnce(httpError(404, 'not_found', 'attachment not found'))
    button('YES, REMOVE')!.click()
    await flushPromises()
    expect(rows()).toHaveLength(1)
    expect(rowText(0)).toContain('attachment not found')
    expect(document.querySelector('[aria-label="Remove receipt receipt.png"]')).not.toBeNull()
  })

  it('saves a leftover failed upload into the open form instead of dropping it', async () => {
    seed({ attachment_count: 0 })
    fetchMock.mockImplementation(async (_url: string, o: { method?: string }) => {
      if (o?.method === 'PUT') return { data: makeEntry({ updated_at: 'T2' }) }
      throw httpError(413, 'too_large', 'attachment too large: the limit is 15 MB per file')
    })
    const w = mountForm({ editId: 'e-1' })
    await flushPromises()
    await pick('rc-file-input', [makeFile('big.pdf', 'application/pdf')])
    expect(rowText(0)).toContain('FAILED')
    await submit()
    expect(w.emitted('saved')).toHaveLength(1)
    expect(w.emitted('close')).toBeUndefined()
    expect(document.querySelector('.ee-note')!.textContent).toBe('Entry saved. 1 of 1 receipt failed to upload.')
    expect(rows()).toHaveLength(1)
  })
})

describe('voided entry', () => {
  it('lists receipts with open links but offers no way to add or remove them', async () => {
    const store = useEarningsStore()
    store.entries = [makeEntry({ status: 'voided', attachment_count: 1 })]
    fetchMock.mockResolvedValueOnce({ data: [makeAttachment()] })
    mountForm({ editId: 'e-1' })
    await flushPromises()

    expect(rows()).toHaveLength(1)
    expect(rows()[0]!.querySelector('a')!.getAttribute('href')).toBe('/api/v1/finance/entries/e-1/attachments/a-1')
    expect(button('TAKE PHOTO')).toBeUndefined()
    expect(button('ADD FILE')).toBeUndefined()
    expect(document.querySelector('[data-testid="rc-dropzone"]')).toBeNull()
    expect(document.querySelector('[data-rc-remove]')).toBeNull()
    expect(document.querySelector('.rc [role="note"]')!.textContent).toMatch(/Voided entry/)
  })
})
