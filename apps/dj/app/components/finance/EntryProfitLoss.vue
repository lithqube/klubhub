<script setup lang="ts">
// Profit/loss panes per gig / per month / per year (FIN-07). Multi-currency
// totals; no conversion. The page owns the source-of-truth for the gig id
// when scope='gig' (it reads from the gigById selector in the store).
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useEarningsStore } from '../../stores/earnings'
import { formatMinor } from '../../utils/money'
import type { Gig } from '../../types/gig'
import type { ProfitLossTotals } from '../../types/finance'

const props = defineProps<{
  /** When true the picker is locked to gig scope and uses this gig id. */
  gigId?: string | null
  gig?: Gig | null
}>()

const store = useEarningsStore()
const { profitLossByScope, profitLossLoading, profitLossError, profitLossScope } = storeToRefs(store)

type Mode = 'gig' | 'month' | 'year'
const mode = ref<Mode>('month')
const localYear = ref(new Date().getUTCFullYear())
const localMonth = ref(new Date().getUTCMonth() + 1)

const MONTHS = [
  'JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN',
  'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC',
]
const yearOptions = computed(() => {
  const list: number[] = []
  for (let y = 2020; y <= localYear.value + 1; y++) list.push(y)
  return list
})

const subtitle = computed(() => {
  if (mode.value === 'gig') return props.gig
    ? `${props.gig.event_name || props.gig.venue || 'Gig'} · ${props.gig.date.slice(0, 10)}`
    : 'Pick a gig'
  if (mode.value === 'year') return `YEAR · ${localYear.value}`
  return `${MONTHS[localMonth.value - 1]} ${localYear.value}`
})

async function refresh(): Promise<void> {
  if (mode.value === 'gig') {
    const id = props.gigId ?? props.gig?.id
    if (id) await store.refreshProfitLoss({ scope: 'gig', gig_id: id })
    else await store.refreshProfitLoss({ scope: 'month', year: localYear.value, month: localMonth.value })
  } else if (mode.value === 'year') {
    await store.refreshProfitLoss({ scope: 'year', year: localYear.value })
  } else {
    await store.refreshProfitLoss({ scope: 'month', year: localYear.value, month: localMonth.value })
  }
}

watch([mode, localYear, localMonth, () => props.gigId, () => props.gig?.id], () => { void refresh() }, { immediate: true })

const rows = computed(() => Object.values((profitLossByScope.value?.data ?? {}) as Record<string, ProfitLossTotals>))
const state = computed<'loading' | 'error' | 'empty' | 'rows'>(() => {
  if (profitLossLoading.value) return 'loading'
  if (profitLossError.value) return 'error'
  if (!rows.value.length) return 'empty'
  return 'rows'
})

const canPickGig = computed(() => Boolean(props.gigId))

watch(profitLossScope, (s) => {
  if ('scope' in s) {
    if (s.scope !== 'gig') {
      mode.value = s.scope
      if (s.scope === 'year') localYear.value = s.year
      if (s.scope === 'month') {
        localYear.value = s.year
        localMonth.value = s.month
      }
    } else {
      mode.value = 'gig'
    }
  }
}, { immediate: true })

defineExpose({ refresh })
</script>

<template>
  <div class="pl">
    <div class="pl-head">
      <span class="section-lbl">PROFIT / LOSS · FIN-07</span>
      <div class="pl-controls">
        <div class="pl-buttons" role="group" aria-label="P&L scope">
          <button
            type="button"
            class="pl-btn"
            :class="{ 'pl-btn-on': mode === 'gig' }"
            :aria-pressed="mode === 'gig'"
            :disabled="!canPickGig"
            @click="mode = 'gig'"
          >GIG</button>
          <button
            type="button"
            class="pl-btn"
            :class="{ 'pl-btn-on': mode === 'month' }"
            :aria-pressed="mode === 'month'"
            @click="mode = 'month'"
          >MONTH</button>
          <button
            type="button"
            class="pl-btn"
            :class="{ 'pl-btn-on': mode === 'year' }"
            :aria-pressed="mode === 'year'"
            @click="mode = 'year'"
          >YEAR</button>
        </div>
        <template v-if="mode === 'month'">
          <select v-model.number="localYear" class="hud-input pl-select" aria-label="Year">
            <option v-for="y in yearOptions" :key="y" :value="y">{{ y }}</option>
          </select>
          <select v-model.number="localMonth" class="hud-input pl-select" aria-label="Month">
            <option v-for="(m, i) in MONTHS" :key="m" :value="i + 1">{{ m }}</option>
          </select>
        </template>
        <template v-else-if="mode === 'year'">
          <select v-model.number="localYear" class="hud-input pl-select" aria-label="Year">
            <option v-for="y in yearOptions" :key="y" :value="y">{{ y }}</option>
          </select>
        </template>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" @click="refresh">REFRESH</button>
      </div>
    </div>

    <div class="pl-sub">{{ subtitle }}</div>

    <div v-if="state === 'loading'" class="glass pl-state" role="status">
      <span class="pl-state-text">Computing profit / loss…</span>
    </div>
    <div v-else-if="state === 'error'" class="glass pl-state" role="alert">
      <span class="pl-state-text">{{ profitLossError?.message || 'Could not load profit/loss.' }}</span>
      <button type="button" class="btn-hud btn-hud-ghost pl-tap" @click="refresh">RETRY</button>
    </div>
    <div v-else-if="state === 'empty'" class="glass pl-state">
      <span class="pl-state-title">NO ACTIVITY</span>
      <span class="pl-state-text">No income or expenses recorded for this scope.</span>
    </div>
    <ul v-else class="glass pl-rows">
      <li v-for="r in rows" :key="r.currency" class="pl-row">
        <span class="pl-cur">{{ r.currency }}</span>
        <span class="pl-amt pl-amt-pos">{{ formatMinor(r.income_minor, r.currency) }}</span>
        <span class="pl-amt pl-amt-neg">-{{ formatMinor(r.expense_minor, r.currency) }}</span>
        <span class="pl-amt pl-amt-net" :class="{ 'pl-amt-loss': r.profit_loss_minor < 0 }">{{ formatMinor(r.profit_loss_minor, r.currency) }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.pl { display: flex; flex-direction: column; gap: 8px; }
.pl-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.pl-controls { display: inline-flex; gap: 6px; align-items: center; flex-wrap: wrap; }
.pl-buttons { display: inline-flex; border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); }
.pl-btn { min-height: 28px; padding: 0 10px; background: transparent; border: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); }
.pl-btn:hover:not(:disabled) { color: var(--color-primary); }
.pl-btn:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 1px; }
.pl-btn-on { color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 10%, transparent); }
.pl-btn:disabled { opacity: .4; cursor: not-allowed; }
.pl-select { height: 28px; padding: 0 8px; font-family: var(--font-command); font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: .04em; }
.pl-sub { font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.pl-state { padding: 14px; display: flex; flex-direction: column; gap: 6px; }
.pl-state-title { font-family: var(--font-command); font-size: 12px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); }
.pl-state-text { font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
.pl-rows { margin: 0; padding: 0; list-style: none; }
.pl-row { display: grid; grid-template-columns: 56px repeat(3, 1fr); align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 10%, transparent); }
.pl-row:last-child { border-bottom: 0; }
.pl-cur { font-family: var(--font-command); font-size: 13px; font-weight: 700; color: var(--color-on-surface); text-align: center; padding: 2px 6px; border: 1px solid color-mix(in srgb, var(--color-on-surface) 20%, transparent); }
.pl-amt { font-family: var(--font-command); font-size: 14px; font-weight: 700; letter-spacing: -.02em; text-align: right; }
.pl-amt-pos { color: var(--color-primary); }
.pl-amt-neg { color: var(--color-error); }
.pl-amt-net { color: var(--color-on-surface); }
.pl-amt-loss { color: var(--color-error); }
@media (max-width: 768px) {
  .pl-row { grid-template-columns: 1fr 1fr; row-gap: 4px; }
  .pl-cur { grid-column: 1 / -1; justify-self: start; }
  .pl-amt { text-align: left; }
  .pl-tap, .pl-btn { min-height: 44px; }
}
</style>
