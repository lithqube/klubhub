// EarningsWorkspace: where the inline entry composer appears. A new entry
// opens at the top of the section (above the filter toolbar and the list); EDIT
// opens the same form in place of that row while the other rows stay visible;
// only one composer is open at a time; focus goes in on open and back out on
// close.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import EarningsWorkspace from '../EarningsWorkspace.vue'
import { useEarningsStore } from '../../../stores/earnings'
import { makeEntry } from './fixtures'

const wrappers: VueWrapper[] = []
function mountWorkspace() {
  const w = mount(EarningsWorkspace, { attachTo: document.body })
  wrappers.push(w)
  return w
}
function seedList() {
  const store = useEarningsStore()
  store.entries = [
    makeEntry({ id: 'a', notes: 'Cable' }),
    makeEntry({ id: 'b', notes: 'Headphones', updated_at: 'T-b' }),
    makeEntry({ id: 'c', notes: 'Strings' }),
  ]
  store.listLoaded = true
  return store
}
const forms = () => document.querySelectorAll('section.ee')
const editButton = (id: string) => document.querySelector(`button[data-edit-for="${id}"]`) as HTMLButtonElement | null

beforeEach(() => {
  setActivePinia(createPinia())
  document.body.innerHTML = ''
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(() => {
  wrappers.splice(0).forEach((w) => w.unmount())
  vi.unstubAllGlobals()
})

describe('new entry', () => {
  it('shows nothing until a composer is requested', async () => {
    seedList()
    mountWorkspace()
    await flushPromises()
    expect(forms()).toHaveLength(0)
    expect(document.querySelectorAll('.el-row')).toHaveLength(3)
  })

  it('opens above the toolbar and the list, which stay visible below it', async () => {
    const store = seedList()
    mountWorkspace()
    store.openCreate('expense')
    await flushPromises()

    const form = document.querySelector('section.ee')!
    expect(form.querySelector('#ee-title')!.textContent).toBe('NEW EXPENSE')
    const toolbar = document.querySelector('.el-toolbar')!
    const list = document.querySelector('.el-rows')!
    expect(form.compareDocumentPosition(toolbar) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(form.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(document.querySelectorAll('.el-row')).toHaveLength(3)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('closes on CANCEL and on Escape, and the store forgets the composer', async () => {
    const store = seedList()
    mountWorkspace()
    store.openCreate('income')
    await flushPromises()
    ;(Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'CANCEL') as HTMLButtonElement).click()
    await flushPromises()
    expect(forms()).toHaveLength(0)
    expect(store.createOpen).toBe(false)

    store.openCreate('income')
    await flushPromises()
    document.querySelector('#ee-amount')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(forms()).toHaveLength(0)
    expect(store.createOpen).toBe(false)
  })

  it('switches kind without stacking a second composer', async () => {
    const store = seedList()
    mountWorkspace()
    store.openCreate('income')
    await flushPromises()
    store.openCreate('expense')
    await flushPromises()
    expect(forms()).toHaveLength(1)
    expect(document.getElementById('ee-title')!.textContent).toBe('NEW EXPENSE')
  })

  it('returns focus to the button that opened it', async () => {
    const store = seedList()
    const opener = document.createElement('button')
    opener.dataset.newEntry = 'expense'
    document.body.appendChild(opener)
    mountWorkspace()
    opener.focus()
    store.openCreate('expense')
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
    document.querySelector('#ee-amount')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await vi.waitFor(() => expect(document.activeElement).toBe(opener))
  })
})

describe('edit', () => {
  it('opens in place of that row; the other rows stay', async () => {
    const store = seedList()
    mountWorkspace()
    store.openEdit('b')
    await flushPromises()

    expect(forms()).toHaveLength(1)
    const inList = document.querySelector('.el-rows section.ee')
    expect(inList).not.toBeNull()
    expect(inList!.querySelector('#ee-title')!.textContent).toBe('EDIT EXPENSE')
    expect((inList!.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('Headphones')
    // The edited row is replaced, the neighbours are untouched.
    const rowTexts = Array.from(document.querySelectorAll('.el-row')).map((r) => r.textContent)
    expect(rowTexts).toHaveLength(2)
    expect(rowTexts[0]).toContain('Cable')
    expect(rowTexts[1]).toContain('Strings')
    const order = Array.from(document.querySelectorAll('.el-rows > li')).map((li) => li.className.includes('el-row-editing') ? 'form' : li.textContent!.includes('Cable') ? 'a' : 'c')
    expect(order).toEqual(['a', 'form', 'c'])
  })

  it('EDIT on a row opens it in place and keeps the focus there', async () => {
    seedList()
    mountWorkspace()
    editButton('b')!.focus()
    editButton('b')!.click()
    await flushPromises()
    expect(document.querySelector('.el-rows section.ee')).not.toBeNull()
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
  })

  it('puts the EDIT button of that row back in focus when the edit closes', async () => {
    seedList()
    mountWorkspace()
    editButton('b')!.focus()
    editButton('b')!.click()
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('#ee-amount')))
    document.querySelector('#ee-amount')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await vi.waitFor(() => expect(document.activeElement).toBe(editButton('b')))
    expect(forms()).toHaveLength(0)
    expect(document.querySelectorAll('.el-row')).toHaveLength(3)
  })

  it('moves the edit to another row when EDIT is pressed there (still one composer)', async () => {
    const store = seedList()
    mountWorkspace()
    store.openEdit('a')
    await flushPromises()
    editButton('c')!.click()
    await flushPromises()
    expect(forms()).toHaveLength(1)
    expect((document.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('Strings')
    expect(editButton('a')).not.toBeNull() // row a is back
  })

  it('replaces a new-entry composer instead of opening beside it', async () => {
    const store = seedList()
    mountWorkspace()
    store.openCreate('income')
    await flushPromises()
    store.openEdit('b')
    await flushPromises()
    expect(forms()).toHaveLength(1)
    expect(document.querySelector('.el-rows section.ee')).not.toBeNull()
    store.openCreate('income')
    await flushPromises()
    expect(forms()).toHaveLength(1)
    expect(document.querySelector('.el-rows section.ee')).toBeNull() // back at the top
  })

  it('opens at the top when the list is not showing that entry (not loaded / filtered out)', async () => {
    const store = seedList()
    store.entries = [makeEntry({ id: 'only', notes: 'Visible' })]
    mountWorkspace()
    store.openEdit('hidden-by-filter')
    await flushPromises()
    expect(forms()).toHaveLength(1)
    expect(document.querySelector('.el-rows section.ee')).toBeNull()
    expect(document.querySelector('section.ee')).not.toBeNull()
  })

  it('keeps an open edit in its row, draft intact, while the list refetches', async () => {
    const store = seedList()
    mountWorkspace()
    store.openEdit('b')
    await flushPromises()
    const notes = document.querySelector('#ee-notes') as HTMLTextAreaElement
    notes.value = 'Mine'
    notes.dispatchEvent(new Event('input', { bubbles: true }))
    store.listLoading = true
    await flushPromises()
    expect(document.querySelector('.el-rows section.ee')).not.toBeNull()
    expect((document.querySelector('#ee-notes') as HTMLTextAreaElement).value).toBe('Mine')
  })
})

describe('saving', () => {
  it('forwards `saved` so the page can refresh totals, then closes', async () => {
    const store = seedList()
    const w = mountWorkspace()
    const saved = makeEntry({ id: 'new', notes: 'Fresh' })
    vi.spyOn(store, 'createEntry').mockResolvedValue(saved)
    store.openCreate('expense')
    await flushPromises()
    const notes = document.querySelector('#ee-notes') as HTMLTextAreaElement
    const amount = document.querySelector('#ee-amount') as HTMLInputElement
    amount.value = '50'; amount.dispatchEvent(new Event('input', { bubbles: true }))
    notes.value = 'Fresh'; notes.dispatchEvent(new Event('input', { bubbles: true }))
    document.querySelector('form.ee-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(w.emitted('saved')).toEqual([[saved]])
    expect(store.createOpen).toBe(false)
    expect(forms()).toHaveLength(0)
  })

  it('relays void and delete requests from the list', async () => {
    seedList()
    const w = mountWorkspace()
    await flushPromises()
    const btn = (label: string) => Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === label) as HTMLButtonElement
    btn('VOID').click()
    btn('DELETE').click()
    expect((w.emitted('void')![0]![0] as { id: string }).id).toBe('a')
    expect((w.emitted('delete')![0]![0] as { id: string }).id).toBe('a')
  })
})
