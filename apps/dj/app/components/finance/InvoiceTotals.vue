<script setup lang="ts">
// Lines, tax breakdown, withholding and legal note. Read-only: for drafts it
// previews the saved figures; for issued documents it is the frozen record.
import { computed } from 'vue'
import type { Invoice, InvoiceLine } from '../../types/finance'
import { bpsToPercent, formatMinor } from '../../utils/money'
import { vatTreatmentLabel } from '../../utils/vatTreatment'

const props = defineProps<{ invoice: Invoice; lines: InvoiceLine[] }>()

const cur = computed(() => props.invoice.currency)
const fmt = (minor: number) => formatMinor(minor, cur.value)
const isDraft = computed(() => props.invoice.status === 'draft')
</script>

<template>
  <section class="tot" aria-labelledby="inv-tot-title">
    <div class="tot-head">
      <h3 id="inv-tot-title" class="section-lbl tot-title">{{ isDraft ? 'TOTALS (SAVED)' : 'TOTALS' }}</h3>
      <span class="section-lbl">{{ vatTreatmentLabel(invoice.vat_treatment) }}</span>
    </div>

    <ul v-if="lines.length" class="tot-lines">
      <li v-for="l in lines" :key="l.id" class="tot-row">
        <span class="tot-desc">{{ l.description }}<template v-if="l.quantity !== 1"> × {{ l.quantity }}</template></span>
        <span class="tot-num">{{ fmt(l.line_total_minor) }}</span>
      </li>
    </ul>

    <dl class="tot-sums">
      <div class="tot-row">
        <dt>Subtotal</dt><dd class="tot-num">{{ fmt(invoice.subtotal_minor) }}</dd>
      </div>
      <template v-if="invoice.tax_breakdown.length">
        <div v-for="row in invoice.tax_breakdown" :key="row.rate_bps" class="tot-row">
          <dt>Tax {{ bpsToPercent(row.rate_bps) }}% on {{ fmt(row.taxable_minor) }}</dt>
          <dd class="tot-num">{{ fmt(row.tax_minor) }}</dd>
        </div>
      </template>
      <div v-else class="tot-row">
        <dt>Tax {{ bpsToPercent(invoice.tax_rate_bps) }}%</dt><dd class="tot-num">{{ fmt(invoice.tax_minor) }}</dd>
      </div>
      <div class="tot-row tot-strong">
        <dt>Total</dt><dd class="tot-num">{{ fmt(invoice.total_minor) }}</dd>
      </div>
      <div v-if="invoice.withholding_rate_bps > 0" class="tot-row">
        <dt>Withheld {{ bpsToPercent(invoice.withholding_rate_bps) }}% of subtotal</dt>
        <dd class="tot-num">−{{ fmt(invoice.withholding_minor) }}</dd>
      </div>
      <div v-if="invoice.withholding_rate_bps > 0" class="tot-row tot-strong">
        <dt>Net payable</dt><dd class="tot-num">{{ fmt(invoice.net_payable_minor) }}</dd>
      </div>
    </dl>

    <div v-if="!isDraft && invoice.tax_note" class="tot-note">
      <div class="section-lbl">LEGAL NOTE</div>
      <p>{{ invoice.tax_note }}</p>
    </div>
    <p v-if="isDraft" class="tot-hint">Figures update when you save the draft.</p>
  </section>
</template>

<style scoped>
.tot { display: flex; flex-direction: column; gap: 8px; }
.tot-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
.tot-title { margin: 0; font-weight: 600; }
.tot-lines { list-style: none; margin: 0; padding: 0 0 6px; border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 12%, transparent); }
.tot-sums { margin: 0; display: flex; flex-direction: column; gap: 2px; }
.tot-row { display: flex; justify-content: space-between; gap: 12px; padding: 3px 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.tot-row dt, .tot-row dd { margin: 0; }
.tot-desc { min-width: 0; color: var(--color-on-surface); }
.tot-num { flex-shrink: 0; font-family: var(--font-command); font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); }
.tot-strong { color: var(--color-on-surface); font-weight: 600; }
.tot-note { padding: 8px 10px; background: var(--color-surface-container-low); }
.tot-note p { margin: 4px 0 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); }
.tot-hint { margin: 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
</style>
