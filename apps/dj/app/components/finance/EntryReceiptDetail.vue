<script setup lang="ts">
// Receipts of one list row, shown inline under it when the paperclip is
// opened. Fetched on demand through the earnings store (the list only carries
// a count); links open in a new tab so the ledger stays where it is.
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { entryAttachmentUrl, useEarningsStore } from '../../stores/earnings'
import { formatBytes, isImageReceipt } from '../../utils/receipts'
import ReceiptLink from './ReceiptLink.vue'
import ReceiptThumb from './ReceiptThumb.vue'

const props = defineProps<{ entryId: string }>()

const store = useEarningsStore()
const { attachments } = storeToRefs(store)
const loading = ref(true)
const error = ref('')
const list = computed(() => attachments.value[props.entryId] ?? [])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    await store.fetchAttachments(props.entryId)
  } catch (e) {
    error.value = (e as { message?: string })?.message || 'Could not load the receipts.'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div class="erd" :aria-busy="loading || undefined">
    <p v-if="loading && !list.length" class="erd-msg" role="status">Loading receipts…</p>
    <div v-else-if="error" class="erd-msg erd-error" role="alert">
      {{ error }}
      <button type="button" class="erd-retry" @click="load">RETRY</button>
    </div>
    <p v-else-if="!list.length" class="erd-msg">No receipts attached.</p>
    <ul v-else class="erd-list" aria-label="Receipts">
      <li v-for="a in list" :key="a.id" class="erd-item">
        <ReceiptThumb :src="entryAttachmentUrl(a.entry_id, a.id, { inline: true })" :image="isImageReceipt({ mime_type: a.mime_type })" />
        <span class="erd-meta">
          <ReceiptLink :attachment="a" />
          <span class="erd-sub">{{ formatBytes(a.size_bytes) }}</span>
        </span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.erd { padding: 8px 0 2px; }
.erd-msg { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-tertiary); }
.erd-error { color: var(--color-error); }
.erd-retry { margin-left: 6px; min-height: 24px; background: none; border: 0; padding: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-primary); text-decoration: underline; }
.erd-list { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 8px 16px; }
.erd-item { display: flex; align-items: center; gap: 8px; min-width: 0; max-width: 100%; }
.erd-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.erd-sub { font-family: var(--font-terminal); font-size: 8px; letter-spacing: .05em; text-transform: uppercase; color: var(--color-tertiary); font-variant-numeric: tabular-nums; }
@media (max-width: 768px) { .erd-retry { min-height: 44px; } }
</style>
