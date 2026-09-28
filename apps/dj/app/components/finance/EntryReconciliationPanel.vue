<script setup lang="ts">
// Reconciliation page panel. Iterates pending reconciliations for all gigs
// the user has touched, surfaces a banner per pending row, and exposes the
// global dialog so the user can resolve anything with one click. The list
// of pending rows comes from the durable GET; the dialog body itself comes
// from the resolution component on gig_id.
//
// This panel never fetches itself; finance.vue owns the fetch so the page
// rebuilds the snapshot after every entry write.
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useEarningsStore } from '../../stores/earnings'
import { formatMinor } from '../../utils/money'
import type { EntryReconciliation } from '../../types/finance'

const emit = defineEmits<{
  open: [rec: EntryReconciliation]
}>()

const store = useEarningsStore()
const { reconciliationByGig } = storeToRefs(store)

const rows = computed<EntryReconciliation[]>(() => {
  const out: EntryReconciliation[] = []
  for (const value of Object.values(reconciliationByGig.value ?? {})) {
    if (value && typeof value === 'object' && 'id' in (value as Record<string, unknown>)) {
      out.push(value as EntryReconciliation)
    }
  }
  return out
})

function copyFor(reason: EntryReconciliation['reason']): string {
  switch (reason) {
    case 'fee_changed': return 'Fee changed'
    case 'currency_changed': return 'Currency changed'
    case 'payment_reversed': return 'Payment reverted'
  }
}

function refresh(): void {
  for (const id of Object.keys(reconciliationByGig.value)) {
    void store.fetchReconciliationForGig(id)
  }
}
defineExpose({ refresh })

function handleOpen(rec: EntryReconciliation): void {
  emit('open', rec)
}
</script>

<template>
  <div v-if="rows.length" class="rec-panel glass" aria-labelledby="rec-panel-title">
    <div class="rec-panel-head">
      <h3 id="rec-panel-title" class="section-lbl">RECONCILIATIONS · FIN-04 / FIN-05</h3>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" @click="refresh">REFRESH</button>
    </div>
    <ul class="rec-rows">
      <li v-for="r in rows" :key="r.id">
        <button
          type="button"
          class="rec-banner"
          @click="handleOpen(r)"
          :aria-label="`Resolve reconciliation for gig ${r.gig_id}: ${copyFor(r.reason)}`"
        >
          <span class="rec-banner-kind">{{ copyFor(r.reason) }}</span>
          <span class="rec-banner-amount">{{ formatMinor(r.gig_amount_minor, r.gig_currency) }}</span>
          <span class="rec-banner-status">{{ r.allowed_actions.length }} ACTIONS AVAILABLE</span>
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.rec-panel { padding: 12px 16px; display: flex; flex-direction: column; gap: 8px; border: 1px dashed color-mix(in srgb, var(--color-primary) 40%, transparent); }
.rec-panel-head { display: flex; align-items: center; justify-content: space-between; }
.rec-rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.rec-banner { display: grid; grid-template-columns: 1fr auto auto; align-items: center; gap: 10px; padding: 8px 12px; width: 100%; background: color-mix(in srgb, var(--color-primary) 6%, transparent); border: 1px solid color-mix(in srgb, var(--color-primary) 30%, transparent); cursor: pointer; font-family: var(--font-command); font-size: 11px; font-weight: 600; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-primary); }
.rec-banner:hover, .rec-banner:focus-visible { background: color-mix(in srgb, var(--color-primary) 12%, transparent); outline: none; }
.rec-banner-amount, .rec-banner-status { font-family: var(--font-terminal); font-size: 8px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
</style>
