<script setup lang="ts">
// Monthly / yearly summary panes (FIN-06). Uses the earnings store; the
// page owns the refresh calls, this component only renders and lets the
// store drive it. Multi-currency totals; never converted (FIN-09).
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useEarningsStore } from '../../stores/earnings'
import { formatMinor } from '../../utils/money'
import type { EntryTotals } from '../../types/finance'

const store = useEarningsStore()
const { summary, summaryLoading, summaryError, summaryScope } = storeToRefs(store)

const today = new Date()
const thisYear = today.getUTCFullYear()
const thisMonth = today.getUTCMonth() + 1

const localMode = ref<'month' | 'year'>('month')
const localYear = ref(thisYear)
const localMonth = ref(thisMonth)

const SCOPES = [
  { value: 'month' as const, label: 'MONTH' },
  { value: 'year' as const, label: 'YEAR' },
]
const MONTHS = [
  'JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN',
  'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC',
]

const yearOptions = computed(() => {
  const from = Math.min(2020, thisYear) - 2
  const to = thisYear + 1
  const list: number[] = []
  for (let y = from; y <= to; y++) list.push(y)
  return list
})

const subtitle = computed(() => {
  if (localMode.value === 'year') return `YEAR · ${localYear.value}`
  return `${MONTHS[localMonth.value - 1]} ${localYear.value}`
})

async function refresh(): Promise<void> {
  if (localMode.value === 'month') {
    await store.refreshSummary({ scope: 'month', year: localYear.value, month: localMonth.value })
  } else {
    await store.refreshSummary({ scope: 'year', year: localYear.value })
  }
}

watch([localMode, localYear, localMonth], () => { void refresh() }, { immediate: true })

const rows = computed(() => Object.values(summary.value as Record<string, EntryTotals>))
const state = computed<'loading' | 'error' | 'rows' | 'empty'>(() => {
  if (summaryLoading.value) return 'loading'
  if (summaryError.value) return 'error'
  if (!rows.value.length) return 'empty'
  return 'rows'
})

function refreshFromLastScope(): void {
  void refresh()
}

// Surface scope so the page can sync the dropdowns when something else
// (filter chip, restore-from-storage) drove the latest refresh.
watch(summaryScope, (v) => {
  if ('scope' in v) {
    localMode.value = v.scope
    if (v.scope === 'year') localYear.value = v.year
    if (v.scope === 'month') {
      localYear.value = v.year
      localMonth.value = v.month
    }
  }
}, { immediate: true })

defineExpose({ refresh })
</script>

<template>
  <div class="es">
    <div class="es-head">
      <span class="section-lbl">SUMMARY · FIN-06</span>
      <div class="es-controls">
        <div class="es-buttons" role="group" aria-label="Summary scope">
          <button
            v-for="s in SCOPES"
            :key="s.value"
            type="button"
            class="es-btn"
            :class="{ 'es-btn-on': localMode === s.value }"
            :aria-pressed="localMode === s.value"
            @click="localMode = s.value"
          >
            {{ s.label }}
          </button>
        </div>
        <select v-model.number="localYear" class="hud-input es-select" :aria-label="'Year'">
          <option v-for="y in yearOptions" :key="y" :value="y">{{ y }}</option>
        </select>
        <select v-if="localMode === 'month'" v-model.number="localMonth" class="hud-input es-select" :aria-label="'Month'">
          <option v-for="(m, i) in MONTHS" :key="m" :value="i + 1">{{ m }}</option>
        </select>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" @click="refreshFromLastScope">REFRESH</button>
      </div>
    </div>

    <div v-if="state === 'loading'" class="glass es-state" role="status">
      <span class="es-state-text">Loading {{ subtitle.toLowerCase() }}…</span>
    </div>
    <div v-else-if="state === 'error'" class="glass es-state" role="alert">
      <span class="es-state-text">{{ summaryError?.message || 'Could not load summary.' }}</span>
      <button type="button" class="btn-hud btn-hud-ghost es-tap" @click="refresh">RETRY</button>
    </div>
    <div v-else-if="state === 'empty'" class="glass es-state">
      <span class="es-state-title">{{ subtitle }}</span>
      <span class="es-state-text">No entries in this period yet.</span>
    </div>
    <ul v-else class="glass es-rows">
      <li v-for="r in rows" :key="r.currency" class="es-row">
        <span class="es-cur">{{ r.currency }}</span>
        <span class="es-amt es-amt-pos">{{ formatMinor(r.income_minor, r.currency) }}</span>
        <span class="es-amt es-amt-neg">-{{ formatMinor(r.expense_minor, r.currency) }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.es { display: flex; flex-direction: column; gap: 8px; }
.es-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.es-controls { display: inline-flex; gap: 6px; align-items: center; flex-wrap: wrap; }
.es-buttons { display: inline-flex; border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); }
.es-btn { min-height: 28px; padding: 0 10px; background: transparent; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.es-btn:hover { color: var(--color-primary); }
.es-btn:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 1px; }
.es-btn-on { color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 10%, transparent); }
.es-select { width: auto; min-width: 76px; height: 28px; padding: 0 8px; font-family: var(--font-command); font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: .04em; }
.es-state { padding: 14px; display: flex; flex-direction: column; gap: 6px; }
.es-state-title { font-family: var(--font-command); font-size: 12px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); }
.es-state-text { font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.es-rows { margin: 0; padding: 0; list-style: none; }
.es-row { display: grid; grid-template-columns: 56px 1fr 1fr; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 10%, transparent); }
.es-row:last-child { border-bottom: 0; }
.es-cur { font-family: var(--font-command); font-size: 13px; font-weight: 700; color: var(--color-on-surface); text-align: center; padding: 2px 6px; border: 1px solid color-mix(in srgb, var(--color-on-surface) 20%, transparent); }
.es-amt { font-family: var(--font-command); font-size: 14px; font-weight: 700; letter-spacing: -.02em; font-variant-numeric: tabular-nums; text-align: right; color: var(--color-on-surface); }
/* Income and expense differ by sign and column; the signature and error colours stay reserved. */
.es-amt-pos, .es-amt-neg { color: var(--color-on-surface); }
@media (max-width: 768px) {
  .es-row { grid-template-columns: 1fr 1fr; row-gap: 4px; }
  .es-cur { grid-column: 1 / -1; justify-self: start; }
  .es-amt { text-align: left; }
  .es-tap, .es-btn { min-height: 44px; }
}
</style>
