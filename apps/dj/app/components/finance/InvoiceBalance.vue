<script setup lang="ts">
// Balance of an issued or paid invoice: OUTSTANDING first, then the chain
// TOTAL · WITHHELD · NET PAYABLE · RECEIVED · PENDING, and a progress bar of
// received against net payable. Every figure comes from the server.
import { computed } from 'vue'
import type { Invoice } from '../../types/finance'
import { formatMinor } from '../../utils/money'

const props = defineProps<{ invoice: Invoice }>()

const cur = computed(() => props.invoice.currency)
const fmt = (minor: number) => formatMinor(minor, cur.value)

const pct = computed(() => {
  const net = props.invoice.net_payable_minor
  if (net <= 0) return 100
  return Math.max(0, Math.min(100, Math.floor((props.invoice.received_minor * 100) / net)))
})

const pendingPct = computed(() => {
  const net = props.invoice.net_payable_minor
  if (net <= 0) return 0
  return Math.max(0, Math.min(100 - pct.value, Math.floor((props.invoice.pending_minor * 100) / net)))
})

const items = computed(() => [
  { label: 'TOTAL', value: fmt(props.invoice.total_minor) },
  { label: 'WITHHELD', value: props.invoice.withholding_minor ? `−${fmt(props.invoice.withholding_minor)}` : fmt(0) },
  { label: 'NET PAYABLE', value: fmt(props.invoice.net_payable_minor) },
  { label: 'RECEIVED', value: fmt(props.invoice.received_minor) },
  { label: 'PENDING', value: fmt(props.invoice.pending_minor) },
])
</script>

<template>
  <section class="bal" aria-labelledby="inv-bal-title">
    <div class="bal-top">
      <div>
        <h3 id="inv-bal-title" class="section-lbl bal-lbl">OUTSTANDING</h3>
        <div class="bal-big" :class="{ 'bal-zero': invoice.outstanding_minor === 0 }">
          {{ fmt(invoice.outstanding_minor) }}
        </div>
      </div>
      <span v-if="invoice.outstanding_minor === 0 && invoice.pending_minor === 0" class="badge-hud badge-ready">SETTLED</span>
    </div>

    <dl class="bal-chain">
      <div v-for="it in items" :key="it.label" class="bal-item">
        <dt class="section-lbl">{{ it.label }}</dt>
        <dd class="bal-val">{{ it.value }}</dd>
      </div>
    </dl>

    <div
      class="bal-track"
      role="progressbar"
      aria-label="Received of net payable"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="pct"
      :aria-valuetext="`${pct}% received`"
    >
      <div class="bal-fill" :style="{ width: pct + '%' }" />
      <div class="bal-pending" :style="{ width: pendingPct + '%' }" />
    </div>
    <p class="bal-meta">{{ pct }}% RECEIVED<template v-if="pendingPct"> · {{ pendingPct }}% PENDING</template></p>
  </section>
</template>

<style scoped>
.bal { display: flex; flex-direction: column; gap: 10px; padding: 12px 14px; background: var(--color-surface-container-low); }
.bal-top { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
.bal-lbl { margin: 0 0 2px; font-weight: 600; }
.bal-big { font-family: var(--font-command); font-size: 28px; font-weight: 700; letter-spacing: -.02em; color: var(--color-primary); line-height: 1.1; }
.bal-zero { color: var(--color-on-surface); }
.bal-chain { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 8px; margin: 0; }
.bal-item { min-width: 0; }
.bal-val { margin: 2px 0 0; font-family: var(--font-command); font-size: 13px; font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); overflow-wrap: anywhere; }
.bal-track { position: relative; display: flex; height: 4px; width: 100%; background: var(--color-surface-container-high); }
.bal-fill { height: 100%; background: var(--color-primary); transition: width .2s; }
.bal-pending { height: 100%; background: repeating-linear-gradient(90deg, var(--color-secondary) 0 4px, transparent 4px 6px); transition: width .2s; }
.bal-meta { margin: 0; font-family: var(--font-terminal); font-size: 8px; letter-spacing: .06em; color: var(--color-tertiary); }
@media (max-width: 768px) {
  .bal-chain { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
