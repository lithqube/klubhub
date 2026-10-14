// Receipt files of one entry form: picked files, their upload state and the
// saved receipts. Before the entry exists, files wait as `queued`; once it
// does, they upload one at a time through the earnings store (the only place
// that talks to the API). A failed file stays in the list with its error so
// it can be retried or removed; nothing is dropped silently.
import { computed, getCurrentInstance, onBeforeUnmount, reactive, ref } from 'vue'
import { useEarningsStore } from '../stores/earnings'
import type { EntryAttachment } from '../types/finance'
import { checkReceiptFile, isImageReceipt, RECEIPT_MAX_FILES } from '../utils/receipts'

export type ReceiptStatus = 'queued' | 'uploading' | 'done' | 'failed' | 'removing'

export interface ReceiptItem {
  key: string
  name: string
  size: number
  mime: string
  status: ReceiptStatus
  /** Server or transport message for a failed upload / removal. */
  error: string
  /** The picked file while it is not on the server yet. */
  file: File | null
  /** Set once the receipt is stored. */
  attachment: EntryAttachment | null
  /** Object URL for a not-yet-uploaded image; revoked when it is no longer needed. */
  preview: string | null
  /** Inline "Remove receipt?" confirmation is showing. */
  confirming: boolean
}

let sequence = 0

function previewFor(file: File): string | null {
  if (!isImageReceipt(file) || typeof URL === 'undefined' || typeof URL.createObjectURL !== 'function') return null
  try { return URL.createObjectURL(file) } catch { return null }
}

function revoke(url: string | null): void {
  if (url && typeof URL !== 'undefined' && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(url)
}

function messageOf(e: unknown, fallback: string): string {
  const m = (e as { message?: unknown } | null)?.message
  return typeof m === 'string' && m ? m : fallback
}

function fromAttachment(a: EntryAttachment): ReceiptItem {
  return {
    key: `saved-${a.id}`, name: a.filename, size: a.size_bytes, mime: a.mime_type, status: 'done',
    error: '', file: null, attachment: a, preview: null, confirming: false,
  }
}

export function useReceiptQueue() {
  const store = useEarningsStore()
  const items = ref<ReceiptItem[]>([])
  /** Pre-check rejections from the last pick (type, size, count). */
  const rejections = ref<string[]>([])
  /** Text for the polite live region. */
  const announcement = ref('')
  const loadError = ref('')
  /** Files that went through an upload attempt in this session. */
  const attempted = ref(0)
  let entryId: string | null = null
  let disposed = false
  let pumping: Promise<void> | null = null

  const count = computed(() => items.value.length)
  const busy = computed(() => items.value.some((i) => i.status === 'uploading' || i.status === 'removing'))
  const queuedCount = computed(() => items.value.filter((i) => i.status === 'queued').length)
  const failedCount = computed(() => items.value.filter((i) => i.status === 'failed').length)
  /** Files that are not safely stored yet. */
  const pendingCount = computed(() => queuedCount.value + failedCount.value)

  function announce(text: string): void {
    announcement.value = text
  }

  function find(key: string): ReceiptItem | undefined {
    return items.value.find((i) => i.key === key)
  }

  /** Points uploads at an entry; from here on picked files upload at once. */
  function bind(id: string | null): void {
    entryId = id
  }

  function addFiles(files: File[] | FileList | null | undefined): number {
    rejections.value = []
    const accepted: File[] = []
    let overflow = 0
    for (const file of Array.from(files ?? [])) {
      const why = checkReceiptFile(file)
      if (why) { rejections.value.push(why); continue }
      if (items.value.length + accepted.length >= RECEIPT_MAX_FILES) { overflow++; continue }
      accepted.push(file)
    }
    if (overflow) {
      rejections.value.push(`An entry holds up to ${RECEIPT_MAX_FILES} receipts: ${overflow} file${overflow === 1 ? '' : 's'} not added.`)
    }
    for (const file of accepted) {
      items.value.push({
        key: `new-${++sequence}`, name: file.name, size: file.size, mime: file.type, status: 'queued',
        error: '', file, attachment: null, preview: previewFor(file), confirming: false,
      })
    }
    if (accepted.length) {
      announce(entryId ? `Uploading ${accepted.length} receipt${accepted.length === 1 ? '' : 's'}.` : `${accepted.length} receipt${accepted.length === 1 ? '' : 's'} added. They upload when the entry is saved.`)
      if (entryId) void pump()
    } else if (rejections.value.length) {
      announce(rejections.value.join(' '))
    }
    return accepted.length
  }

  async function upload(item: ReceiptItem): Promise<void> {
    const id = entryId
    if (!id || !item.file) return
    item.status = 'uploading'
    item.error = ''
    attempted.value++
    try {
      const saved = await store.uploadAttachment(id, item.file)
      if (disposed) return
      revoke(item.preview)
      Object.assign(item, { attachment: saved, status: 'done', name: saved.filename, size: saved.size_bytes, mime: saved.mime_type, file: null, preview: null })
      announce(`${saved.filename} saved.`)
    } catch (e) {
      if (disposed) return
      item.status = 'failed'
      item.error = messageOf(e, 'Could not upload this file.')
      announce(`${item.name} did not upload. ${item.error}`)
    }
  }

  async function run(): Promise<void> {
    while (!disposed && entryId) {
      const next = items.value.find((i) => i.status === 'queued')
      if (!next) break
      await upload(next)
    }
  }

  /** Uploads every queued file, one after the other. */
  function pump(): Promise<void> {
    if (!pumping) pumping = run().finally(() => { pumping = null })
    return pumping
  }

  /** Create flow: the entry now exists, so send what was waiting. */
  async function uploadQueued(id: string): Promise<{ failed: number; total: number }> {
    bind(id)
    await pump()
    return { failed: failedCount.value, total: attempted.value }
  }

  function retry(key: string): void {
    const item = find(key)
    if (!item || item.status !== 'failed' || !item.file) return
    item.status = 'queued'
    item.error = ''
    if (entryId) void pump()
  }

  /** Loads the entry's stored receipts (edit mode), keeping files still waiting. */
  async function loadSaved(id: string): Promise<void> {
    bind(id)
    loadError.value = ''
    try {
      const list = await store.fetchAttachments(id)
      if (disposed || entryId !== id) return
      items.value = [...list.map(fromAttachment), ...items.value.filter((i) => !i.attachment)]
    } catch (e) {
      if (!disposed) loadError.value = messageOf(e, 'Could not load the receipts.')
    }
  }

  /** Drops a file that is not stored (queued or failed): nothing to confirm. */
  function discard(key: string): void {
    const idx = items.value.findIndex((i) => i.key === key)
    const item = items.value[idx]
    if (!item || item.attachment) return
    revoke(item.preview)
    items.value.splice(idx, 1)
    announce(`${item.name} removed.`)
  }

  /** Remove button: a queued/failed file goes at once, a stored one asks first. */
  function remove(key: string): void {
    const item = find(key)
    if (!item || item.status === 'uploading' || item.status === 'removing') return
    if (item.attachment) item.confirming = true
    else discard(key)
  }

  function cancelRemove(key: string): void {
    const item = find(key)
    if (item) item.confirming = false
  }

  async function confirmRemove(key: string): Promise<void> {
    const item = find(key)
    const id = entryId
    if (!item?.attachment || !id) return
    item.confirming = false
    item.status = 'removing'
    item.error = ''
    try {
      await store.deleteAttachment(id, item.attachment.id)
      if (disposed) return
      const idx = items.value.findIndex((i) => i.key === key)
      if (idx !== -1) items.value.splice(idx, 1)
      announce(`${item.name} removed.`)
    } catch (e) {
      if (disposed) return
      item.status = 'done'
      item.error = messageOf(e, 'Could not remove this receipt.')
      announce(`${item.name} was not removed. ${item.error}`)
    }
  }

  function dismissRejections(): void {
    rejections.value = []
  }

  function dispose(): void {
    disposed = true
    for (const i of items.value) revoke(i.preview)
  }

  /** Clears everything for a fresh form (another entry, or a new one). */
  function reset(): void {
    for (const i of items.value) revoke(i.preview)
    items.value = []
    rejections.value = []
    announcement.value = ''
    loadError.value = ''
    attempted.value = 0
    entryId = null
  }

  if (getCurrentInstance()) onBeforeUnmount(dispose)

  return reactive({
    items, rejections, announcement, loadError, attempted,
    count, busy, queuedCount, failedCount, pendingCount,
    bind, addFiles, uploadQueued, retry, loadSaved, remove, discard, cancelRemove, confirmRemove,
    dismissRejections, reset, dispose,
  })
}

export type ReceiptQueue = ReturnType<typeof useReceiptQueue>
