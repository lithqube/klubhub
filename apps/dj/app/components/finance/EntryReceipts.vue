<script setup lang="ts">
// RECEIPTS section of the entry form: two ways to pick (phone camera, files),
// a drop zone for the desktop, and the list of files with their upload state.
// All state lives in the queue (composables/useReceiptQueue); this component
// only renders it and forwards picks. A voided entry shows the list read-only.
import { nextTick, ref } from 'vue'
import { useDropZone } from '@vueuse/core'
import { Camera, Paperclip } from 'lucide-vue-next'
import { entryAttachmentUrl } from '../../stores/earnings'
import type { ReceiptItem, ReceiptQueue } from '../../composables/useReceiptQueue'
import { formatBytes, isImageReceipt, RECEIPT_ACCEPT, RECEIPT_HINT, RECEIPT_MAX_FILES } from '../../utils/receipts'
import ReceiptLink from './ReceiptLink.vue'
import ReceiptThumb from './ReceiptThumb.vue'

const props = defineProps<{
  q: ReceiptQueue
  /** Voided entry: files can be opened but not added or removed. */
  readOnly?: boolean
  /** The entry is being saved: no picking meanwhile. */
  disabled?: boolean
}>()

const root = ref<HTMLElement | null>(null)
const photoInput = ref<HTMLInputElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const dropEl = ref<HTMLElement | null>(null)

const { isOverDropZone } = useDropZone(dropEl, {
  onDrop: (files) => { if (!props.readOnly && !props.disabled && files?.length) props.q.addFiles(files) },
})

function onPick(e: Event): void {
  const input = e.target as HTMLInputElement
  // Copy first: clearing the input empties the live FileList.
  const files = Array.from(input.files ?? [])
  input.value = ''
  if (files.length) props.q.addFiles(files)
}

function statusLabel(it: ReceiptItem): string {
  switch (it.status) {
    case 'queued': return 'QUEUED'
    case 'uploading': return 'UPLOADING…'
    case 'removing': return 'REMOVING…'
    case 'failed': return 'FAILED'
    default: return 'SAVED'
  }
}

function thumbSrc(it: ReceiptItem): string | null {
  if (it.attachment) return entryAttachmentUrl(it.attachment.entry_id, it.attachment.id, { inline: true })
  return it.preview
}

function focusIn(selector: string): boolean {
  const el = root.value?.querySelector<HTMLElement>(selector)
  el?.focus()
  return !!el
}

async function ask(it: ReceiptItem): Promise<void> {
  const stored = !!it.attachment
  props.q.remove(it.key)
  await nextTick()
  // A stored file asks first and lands on the safe choice; a waiting one is gone.
  if (stored) focusIn(`[data-rc-keep="${it.key}"]`)
  else focusIn('.rc-pick')
}

async function keep(it: ReceiptItem): Promise<void> {
  props.q.cancelRemove(it.key)
  await nextTick()
  focusIn(`[data-rc-remove="${it.key}"]`)
}

async function confirm(it: ReceiptItem): Promise<void> {
  await props.q.confirmRemove(it.key)
  await nextTick()
  // The row is gone on success; on failure the REMOVE button is back.
  if (!focusIn(`[data-rc-remove="${it.key}"]`)) focusIn('.rc-pick')
}

function cancelAllConfirms(): void {
  for (const it of props.q.items) it.confirming = false
}
</script>

<template>
  <div ref="root" class="rc" role="group" aria-labelledby="rc-title" aria-describedby="rc-hint">
    <div class="rc-head">
      <h3 id="rc-title" class="rc-legend">RECEIPTS</h3>
      <span v-if="q.count" class="rc-count">{{ q.count }}/{{ RECEIPT_MAX_FILES }}</span>
    </div>

    <template v-if="!readOnly">
      <div class="rc-actions">
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm rc-pick rc-pick-photo" :disabled="disabled" @click="photoInput?.click()">
          <Camera :size="14" aria-hidden="true" /> TAKE PHOTO
        </button>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm rc-pick rc-pick-file" :disabled="disabled" @click="fileInput?.click()">
          <Paperclip :size="14" aria-hidden="true" /> ADD FILE
        </button>
      </div>
      <!-- Opens the camera app directly on a phone; a plain file picker on desktop. -->
      <input
        ref="photoInput"
        type="file"
        class="rc-input"
        data-testid="rc-photo-input"
        accept="image/*"
        capture="environment"
        tabindex="-1"
        aria-hidden="true"
        hidden
        @change="onPick"
      >
      <input
        ref="fileInput"
        type="file"
        class="rc-input"
        data-testid="rc-file-input"
        :accept="RECEIPT_ACCEPT"
        multiple
        tabindex="-1"
        aria-hidden="true"
        hidden
        @change="onPick"
      >
      <div ref="dropEl" class="upload-zone rc-drop" :class="{ 'rc-drop-on': isOverDropZone }" data-testid="rc-dropzone">
        <span class="rc-drop-text">DROP FILES HERE</span>
      </div>
      <p id="rc-hint" class="rc-msg">{{ RECEIPT_HINT }}</p>
    </template>
    <p v-else id="rc-hint" class="rc-msg" role="note">Voided entry: receipts can be opened but not added or removed.</p>

    <ul v-if="q.rejections.length" class="rc-reject" role="alert">
      <li v-for="(r, i) in q.rejections" :key="i" class="rc-msg rc-error">{{ r }}</li>
    </ul>
    <p v-if="q.loadError" class="rc-msg rc-error" role="alert">{{ q.loadError }}</p>

    <ul v-if="q.items.length" class="rc-list" aria-label="Receipts" @keydown.esc.stop="cancelAllConfirms">
      <li v-for="it in q.items" :key="it.key" class="rc-item" :class="`rc-item-${it.status}`" :aria-busy="it.status === 'uploading' || it.status === 'removing' || undefined">
        <ReceiptThumb :src="thumbSrc(it)" :image="isImageReceipt({ type: it.mime, name: it.name })" />
        <div class="rc-meta">
          <ReceiptLink v-if="it.attachment" :attachment="it.attachment" />
          <span v-else class="rc-name">{{ it.name }}</span>
          <span class="rc-sub">
            <span class="rc-size">{{ formatBytes(it.size) }}</span>
            <span aria-hidden="true"> · </span>
            <span class="rc-status" :class="{ 'rc-status-failed': it.status === 'failed' }">{{ statusLabel(it) }}</span>
          </span>
          <span v-if="it.error" class="rc-msg rc-error">{{ it.error }}</span>
        </div>
        <div v-if="!readOnly" class="rc-ops">
          <template v-if="it.confirming">
            <div class="rc-confirm" role="group" :aria-label="`Remove receipt ${it.name}?`">
              <span class="rc-confirm-q">REMOVE RECEIPT?</span>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs rc-op" :data-rc-keep="it.key" @click="keep(it)">KEEP</button>
              <button type="button" class="btn-hud btn-hud-error btn-hud-xs rc-op" @click="confirm(it)">YES, REMOVE</button>
            </div>
          </template>
          <template v-else>
            <button v-if="it.status === 'failed' && it.file" type="button" class="btn-hud btn-hud-ghost btn-hud-xs rc-op" :aria-label="`Retry upload of ${it.name}`" @click="q.retry(it.key)">RETRY</button>
            <button
              v-if="it.status !== 'uploading' && it.status !== 'removing'"
              type="button"
              class="btn-hud btn-hud-ghost btn-hud-xs rc-op"
              :data-rc-remove="it.key"
              :aria-label="`Remove receipt ${it.name}`"
              @click="ask(it)"
            >REMOVE</button>
          </template>
        </div>
      </li>
    </ul>
    <p v-else-if="readOnly" class="rc-msg">No receipts were attached.</p>

    <p class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ q.announcement }}</p>
  </div>
</template>

<style scoped>
.rc { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
.rc-head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
/* Section legend: one step above the tertiary readouts under it (DESIGN.md, form sections). */
.rc-legend { margin: 0; font-family: var(--font-terminal); font-size: 9px; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--color-on-surface); }
.rc-count { font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-tertiary); font-variant-numeric: tabular-nums; }
.rc-msg { margin: 0; font-family: var(--font-data); font-size: 11px; line-height: 1.4; color: var(--color-tertiary); }
.rc-error { color: var(--color-error); }
.rc-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.rc-pick { gap: 6px; }
.rc-pick:disabled { opacity: .45; cursor: not-allowed; }
.rc .rc-drop.upload-zone { padding: 12px; flex-direction: row; gap: 8px; cursor: default; border-color: color-mix(in srgb, var(--color-primary) 30%, transparent); }
.rc .rc-drop.upload-zone::before { display: none; }
.rc .rc-drop.upload-zone:hover { box-shadow: none; background: transparent; border-color: color-mix(in srgb, var(--color-primary) 30%, transparent); }
.rc .rc-drop.rc-drop-on { border-style: solid; border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 8%, transparent); }
.rc-drop-text { font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.rc-reject { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 3px; }
.rc-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
.rc-item { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 8px 0; border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.rc-item:first-child { padding-top: 0; }
.rc-item:last-child { border-bottom: 0; padding-bottom: 0; }
.rc-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.rc-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface); }
.rc-sub { font-family: var(--font-terminal); font-size: 8px; letter-spacing: .05em; text-transform: uppercase; color: var(--color-tertiary); font-variant-numeric: tabular-nums; }
.rc-status-failed { color: var(--color-error); font-weight: 700; }
.rc-ops { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
.rc-confirm { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
.rc-confirm-q { font-family: var(--font-terminal); font-size: 8px; font-weight: 700; letter-spacing: .06em; color: var(--color-on-surface); }
@media (max-width: 768px) {
  .rc-pick { flex: 1 1 0; min-height: 44px; }
  .rc-op { min-height: 44px; }
  .rc-item { grid-template-columns: 44px minmax(0, 1fr); }
  .rc-ops { grid-column: 1 / -1; justify-content: flex-start; }
}
/* No drag and drop on a touch screen: the two buttons are the way in. */
@media (pointer: coarse) { .rc .rc-drop.upload-zone { display: none; } }
</style>
