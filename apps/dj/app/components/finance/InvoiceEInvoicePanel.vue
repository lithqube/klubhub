<script setup lang="ts">
// "E-invoice": the structured, machine-readable version of a numbered invoice
// (Factur-X hybrid PDF, XRechnung XML). The server validates before it ships,
// so this panel asks first and lists what is missing instead of offering a
// download that would be refused. The file itself is a plain link to a GET
// that answers with an attachment; it never passes through JS memory.
import { ref } from 'vue'
import type { EInvoiceCheck, EInvoiceFormat, Invoice } from '../../types/finance'
import { invoiceEInvoiceUrl, useInvoiceStore } from '../../stores/invoice'
import { invoiceNumberLabel } from '../../utils/invoiceDisplay'

const props = defineProps<{ invoice: Invoice }>()
const store = useInvoiceStore()

const FORMATS: { id: EInvoiceFormat; label: string; hint: string }[] = [
  { id: 'facturx', label: 'FACTUR-X', hint: 'PDF with the data embedded (ZUGFeRD)' },
  { id: 'xrechnung-cii', label: 'XRECHNUNG CII', hint: 'XML for German public buyers' },
  { id: 'xrechnung-ubl', label: 'XRECHNUNG UBL', hint: 'XML for German public buyers' },
]

const open = ref(false)
const format = ref<EInvoiceFormat>('facturx')
const loading = ref(false)
const unavailable = ref(false)
const failure = ref('')
const check = ref<EInvoiceCheck | null>(null)
let seq = 0

async function run(): Promise<void> {
  const mine = ++seq
  loading.value = true
  failure.value = ''
  check.value = null
  try {
    const res = await store.fetchEInvoiceCheck(props.invoice.id, format.value)
    if (mine !== seq) return
    unavailable.value = res === null
    check.value = res
  } catch (e) {
    if (mine !== seq) return
    failure.value = e instanceof Error ? e.message : 'Could not check the invoice.'
  } finally {
    if (mine === seq) loading.value = false
  }
}

function toggle(): void {
  open.value = !open.value
  if (open.value) void run()
}

function pick(f: EInvoiceFormat): void {
  format.value = f
  void run()
}
</script>

<template>
  <section class="iep" :aria-label="`E-invoice for ${invoiceNumberLabel(invoice)}`">
    <button type="button" class="iep-toggle" :aria-expanded="open" @click="toggle">
      <span class="section-lbl">E-INVOICE</span>
      <span class="iep-chev" aria-hidden="true">{{ open ? '−' : '+' }}</span>
    </button>

    <div v-if="open" class="iep-body">
      <div class="iep-formats" role="radiogroup" aria-label="E-invoice format">
        <button
          v-for="f in FORMATS"
          :key="f.id"
          type="button"
          role="radio"
          class="iep-format"
          :class="{ 'iep-format-on': format === f.id }"
          :aria-checked="format === f.id"
          :title="f.hint"
          @click="pick(f.id)"
        >
          {{ f.label }}
        </button>
      </div>

      <p v-if="loading" class="iep-note" role="status">Checking…</p>
      <p v-else-if="unavailable" class="iep-note" role="status">
        E-invoice export isn't enabled on this server.
      </p>
      <p v-else-if="failure" class="iep-error" role="alert">{{ failure }}</p>

      <template v-else-if="check">
        <div v-if="check.ready" class="iep-ready">
          <p class="iep-note" role="status">Passes every EN 16931 rule check.</p>
          <a
            class="btn-hud btn-hud-sm iep-download"
            :href="invoiceEInvoiceUrl(invoice.id, format)"
            rel="noopener"
            :aria-label="`Download ${format} e-invoice for ${invoiceNumberLabel(invoice)}`"
          >
            DOWNLOAD
          </a>
        </div>
        <div v-else>
          <p class="iep-note" role="status">Not ready to export. Fix these on the invoice, supplier or customer:</p>
          <ul class="iep-problems">
            <li v-for="p in check.problems" :key="p.field + p.message">
              <code>{{ p.field }}</code>
              <span>{{ p.message }}</span>
            </li>
          </ul>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.iep { border: 1px dashed var(--color-outline-variant); }
.iep-toggle { display: flex; width: 100%; align-items: center; justify-content: space-between; padding: 8px 12px; background: none; border: 0; cursor: pointer; color: var(--color-on-surface); min-height: 36px; }
.iep-chev { font-family: var(--font-terminal); font-size: 14px; }
.iep-body { display: flex; flex-direction: column; gap: 10px; padding: 0 12px 12px; }
.iep-formats { display: flex; flex-wrap: wrap; gap: 6px; }
.iep-format { padding: 6px 10px; background: none; border: 1px dashed var(--color-outline-variant); cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); min-height: 32px; }
.iep-format-on { border-style: solid; border-color: var(--color-primary); color: var(--color-primary); }
.iep-note { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.iep-error { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.iep-ready { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.iep-download { display: inline-flex; align-items: center; text-decoration: none; }
.iep-problems { margin: 8px 0 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 6px; }
.iep-problems li { display: flex; flex-direction: column; gap: 2px; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
.iep-problems code { font-family: var(--font-terminal); font-size: 9px; letter-spacing: .04em; color: var(--color-on-surface-variant); }
@media (max-width: 768px) {
  .iep-toggle, .iep-format, .iep-download { min-height: 44px; }
}
</style>
