<script setup lang="ts">
// "Archive": the files the server keeps for an issued invoice — the PDF as
// issued, the validated e-invoice XML and its validation report — each with a
// checksum so it can be shown to be unchanged. Documents created later for an
// older invoice say so, because they were rendered then, not at issue.
import { computed, ref } from 'vue'
import type { ArchivedDocument, ArchivedDocumentKind, Invoice } from '../../types/finance'
import { documentDownloadUrl, useInvoiceStore } from '../../stores/invoice'
import { invoiceNumberLabel, shortDate } from '../../utils/invoiceDisplay'
import { formatBytes } from '../../utils/receipts'

const props = defineProps<{ invoice: Invoice }>()
const store = useInvoiceStore()

const KIND_ORDER: ArchivedDocumentKind[] = ['invoice_pdf', 'einvoice_xml', 'validation_report', 'general']

const open = ref(false)
const loading = ref(false)
const unavailable = ref(false)
const failure = ref('')
const docs = ref<ArchivedDocument[] | null>(null)
let seq = 0

const sorted = computed(() =>
  [...(docs.value ?? [])].sort(
    (a, b) => KIND_ORDER.indexOf(a.kind) - KIND_ORDER.indexOf(b.kind) || b.version - a.version,
  ),
)

async function load(): Promise<void> {
  const mine = ++seq
  loading.value = true
  failure.value = ''
  try {
    const res = await store.fetchInvoiceDocuments(props.invoice.id)
    if (mine !== seq) return
    unavailable.value = res === null
    docs.value = res
  } catch (e) {
    if (mine !== seq) return
    failure.value = e instanceof Error ? e.message : 'Could not load the archive.'
  } finally {
    if (mine === seq) loading.value = false
  }
}

function toggle(): void {
  open.value = !open.value
  if (open.value) void load()
}

function label(d: ArchivedDocument): string {
  switch (d.kind) {
    case 'invoice_pdf': return props.invoice.kind === 'credit_note' ? 'CREDIT NOTE PDF' : 'INVOICE PDF'
    case 'einvoice_xml': return 'E-INVOICE XML'
    case 'validation_report': return 'VALIDATION REPORT'
    default: return 'DOCUMENT'
  }
}
const rendered = (d: ArchivedDocument) => d.uploaded_by === 'backfill'
</script>

<template>
  <section class="iar" :aria-label="`Archive for ${invoiceNumberLabel(invoice)}`">
    <button type="button" class="iar-toggle" :aria-expanded="open" @click="toggle">
      <span class="section-lbl">ARCHIVE</span>
      <span class="iar-chev" aria-hidden="true">{{ open ? '−' : '+' }}</span>
    </button>

    <div v-if="open" class="iar-body">
      <p v-if="loading" class="iar-note" role="status">Loading…</p>
      <p v-else-if="unavailable" class="iar-note" role="status">Archiving isn't enabled on this server.</p>
      <div v-else-if="failure" role="alert">
        <p class="iar-error">{{ failure }}</p>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" @click="load">RETRY</button>
      </div>
      <p v-else-if="docs && !docs.length" class="iar-note" role="status">Nothing is archived for this document.</p>

      <ul v-else-if="docs" class="iar-list">
        <li v-for="d in sorted" :key="d.id" class="iar-item" :class="{ 'iar-old': !d.is_current }">
          <div class="iar-main">
            <span class="iar-kind">{{ label(d) }}</span>
            <span v-if="d.version > 1" class="iar-tag">V{{ d.version }}</span>
            <span v-if="!d.is_current" class="iar-tag">SUPERSEDED</span>
            <span
              v-if="rendered(d)"
              class="iar-tag iar-tag-later"
              title="Rendered later from the stored invoice. It is not the file produced when the invoice was issued."
            >RENDERED LATER</span>
          </div>
          <div class="iar-meta">
            <span class="iar-file">{{ d.filename }}</span>
            <span>{{ formatBytes(d.size_bytes) }}</span>
            <span>{{ shortDate(d.created_at) }}</span>
            <span class="iar-sum" :title="`SHA-256 ${d.checksum_sha256}`">SHA-256 {{ d.checksum_sha256.slice(0, 12) }}…</span>
          </div>
          <a
            class="btn-hud btn-hud-ghost btn-hud-sm iar-download"
            :href="documentDownloadUrl(d.id)"
            :download="d.filename"
            rel="noopener"
            :aria-label="`Download ${label(d)} ${d.filename}`"
          >
            DOWNLOAD
          </a>
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.iar { border: 1px dashed var(--color-outline-variant); }
.iar-toggle { display: flex; width: 100%; align-items: center; justify-content: space-between; padding: 8px 12px; background: none; border: 0; cursor: pointer; color: var(--color-on-surface); min-height: 36px; }
.iar-chev { font-family: var(--font-terminal); font-size: 14px; }
.iar-body { display: flex; flex-direction: column; gap: 10px; padding: 0 12px 12px; }
.iar-note { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.iar-error { margin: 0 0 6px; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.iar-list { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 8px; }
.iar-item { display: grid; grid-template-columns: 1fr auto; gap: 4px 12px; align-items: center; padding: 8px 0; border-top: 1px dashed var(--color-outline-variant); }
.iar-item:first-child { border-top: 0; padding-top: 0; }
.iar-old { opacity: .65; }
.iar-main { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
.iar-kind { font-family: var(--font-terminal); font-size: 10px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface); }
.iar-tag { font-family: var(--font-terminal); font-size: 9px; letter-spacing: .06em; padding: 1px 5px; border: 1px dashed var(--color-outline-variant); color: var(--color-on-surface-variant); }
.iar-tag-later { border-style: solid; color: var(--color-on-surface); cursor: help; }
.iar-meta { grid-column: 1; display: flex; flex-wrap: wrap; gap: 2px 12px; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.iar-file { overflow-wrap: anywhere; }
.iar-sum { font-family: var(--font-terminal); font-size: 9px; letter-spacing: .04em; }
.iar-download { grid-row: 1 / span 2; grid-column: 2; display: inline-flex; align-items: center; text-decoration: none; }
@media (max-width: 768px) {
  .iar-toggle, .iar-download { min-height: 44px; }
  .iar-item { grid-template-columns: 1fr; }
  .iar-download { grid-row: auto; grid-column: 1; justify-self: start; }
}
</style>
