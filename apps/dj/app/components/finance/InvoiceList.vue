<script setup lang="ts">
// Invoice list with status chips. Rows open the detail sheet; the list
// itself never fetches (the page does), so it renders whatever the store has.
import { computed, nextTick, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Plus } from 'lucide-vue-next'
import type { Invoice, InvoiceListFilter } from '../../types/finance'
import { useInvoiceStore } from '../../stores/invoice'
import { formatMinor } from '../../utils/money'
import {
  DISPLAY_STATUS_ACCENT,
  DISPLAY_STATUS_BADGE,
  LIST_FILTERS,
  invoiceDisplayStatus,
  invoiceNumberLabel,
  partyDisplayName,
  shortDate,
  signedTotal,
  statusLabel,
  todayIso,
} from '../../utils/invoiceDisplay'

const store = useInvoiceStore()
const { invoices, filteredInvoices, filterCounts, filter, listLoaded, listError, disabled } = storeToRefs(store)

const chipEls = ref<HTMLButtonElement[]>([])
const today = computed(() => todayIso())

const state = computed<'disabled' | 'loading' | 'error' | 'empty' | 'filter-empty' | 'rows'>(() => {
  if (disabled.value) return 'disabled'
  if (listError.value && !listLoaded.value) return 'error'
  // Before the first answer (including the SSR pass) show the skeleton, never "empty".
  if (!listLoaded.value) return 'loading'
  if (!invoices.value.length) return 'empty'
  if (!filteredInvoices.value.length) return 'filter-empty'
  return 'rows'
})

const filterLabel = computed(() => LIST_FILTERS.find((f) => f.value === filter.value)?.label ?? '')

function select(f: InvoiceListFilter): void {
  store.setFilter(f)
}

/** Roving focus for the radiogroup: arrows move and select, like native radios. */
function onChipKey(e: KeyboardEvent, index: number): void {
  const n = LIST_FILTERS.length
  let next = -1
  if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = (index + 1) % n
  else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = (index - 1 + n) % n
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = n - 1
  if (next < 0) return
  e.preventDefault()
  select(LIST_FILTERS[next]!.value)
  void nextTick(() => chipEls.value[next]?.focus())
}

function rowDate(inv: Invoice): string {
  return shortDate(inv.issued_at ?? inv.supply_date ?? inv.created_at)
}

function showsBalance(inv: Invoice): boolean {
  return inv.kind === 'invoice' && (inv.status === 'issued' || inv.status === 'paid')
}

function rowLabel(inv: Invoice): string {
  const parts = [
    inv.invoice_number ?? (inv.kind === 'credit_note' ? 'Credit note draft' : 'Draft'),
    inv.kind === 'credit_note' ? 'credit note' : '',
    partyDisplayName(inv.customer) || 'No customer',
    rowDate(inv),
    `total ${formatMinor(signedTotal(inv), inv.currency)}`,
    showsBalance(inv) ? `outstanding ${formatMinor(inv.outstanding_minor, inv.currency)}` : '',
    statusLabel(inv, today.value),
  ]
  return parts.filter(Boolean).join(', ')
}
</script>

<template>
  <div class="il">
    <div
      role="radiogroup"
      aria-label="Filter invoices by status"
      class="il-chips"
    >
      <button
        v-for="(f, i) in LIST_FILTERS"
        :key="f.value"
        ref="chipEls"
        type="button"
        role="radio"
        class="il-chip"
        :class="{ 'il-chip-on': filter === f.value }"
        :aria-checked="filter === f.value"
        :tabindex="filter === f.value ? 0 : -1"
        :disabled="state === 'disabled'"
        @click="select(f.value)"
        @keydown="onChipKey($event, i)"
      >
        {{ f.label }}<span class="il-count">{{ filterCounts[f.value] ?? 0 }}</span>
      </button>
    </div>

    <div v-if="state === 'disabled'" class="glass il-state" role="status">
      <p class="il-state-title">INVOICING OFF</p>
      <p class="il-state-text">Invoicing isn't enabled on this server.</p>
    </div>

    <div v-else-if="state === 'loading'" class="glass" aria-busy="true" aria-label="Loading invoices">
      <div v-for="n in 3" :key="n" class="il-skel">
        <span class="il-skel-bar" style="width:72px;" />
        <span class="il-skel-bar" style="flex:1;" />
        <span class="il-skel-bar" style="width:64px;" />
      </div>
    </div>

    <div v-else-if="state === 'error'" class="glass il-state" role="alert">
      <p class="il-state-title">COULD NOT LOAD INVOICES</p>
      <p class="il-state-text">{{ listError?.message }}</p>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm il-tap" @click="store.fetchInvoices()">RETRY</button>
    </div>

    <div v-else-if="state === 'empty'" class="glass hud-card il-empty">
      <p class="il-state-title">NO INVOICES YET</p>
      <p class="il-state-text">No invoices yet. Bill a played gig in two clicks.</p>
      <button type="button" class="btn-hud btn-hud-cta il-tap" @click="store.openCreate()">
        <Plus style="width:12px;height:12px;" aria-hidden="true" />
        CREATE INVOICE
      </button>
    </div>

    <div v-else-if="state === 'filter-empty'" class="glass il-state" role="status">
      <p class="il-state-text">No {{ filterLabel }} invoices.</p>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm il-tap" @click="select('all')">SHOW ALL</button>
    </div>

    <ul v-else class="glass il-rows" aria-label="Invoices">
      <li v-for="inv in filteredInvoices" :key="inv.id">
        <button
          type="button"
          class="il-row"
          :class="DISPLAY_STATUS_ACCENT[invoiceDisplayStatus(inv, today)]"
          :aria-label="rowLabel(inv)"
          @click="store.openDetail(inv.id)"
        >
          <span class="il-num">{{ invoiceNumberLabel(inv) }}</span>
          <span class="il-main">
            <span class="il-client">{{ partyDisplayName(inv.customer) || 'NO CUSTOMER' }}</span>
            <span class="il-meta">{{ rowDate(inv) }}<template v-if="inv.kind === 'credit_note'"> · CREDIT NOTE</template></span>
          </span>
          <span class="il-money">
            <span class="il-total" :class="{ 'il-neg': inv.kind === 'credit_note' }">{{ formatMinor(signedTotal(inv), inv.currency) }}</span>
            <span v-if="showsBalance(inv)" class="il-meta">
              {{ inv.outstanding_minor > 0 ? `${formatMinor(inv.outstanding_minor, inv.currency)} DUE` : 'NOTHING DUE' }}
            </span>
          </span>
          <span class="badge-hud il-badge" :class="DISPLAY_STATUS_BADGE[invoiceDisplayStatus(inv, today)]">{{ statusLabel(inv, today) }}</span>
        </button>
      </li>
    </ul>
    <p v-if="listError && listLoaded" class="il-inline-error" role="alert">
      Could not refresh: {{ listError.message }}
      <button type="button" class="il-retry" @click="store.fetchInvoices()">RETRY</button>
    </p>
  </div>
</template>

<style scoped>
.il { display: flex; flex-direction: column; gap: 8px; }
.il-chips { display: flex; flex-wrap: wrap; gap: 4px; }
.il-chip { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; cursor: pointer; background: transparent; border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); transition: color .15s, border-color .15s; }
.il-chip:hover { color: var(--color-primary); }
.il-chip:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 1px; }
.il-chip-on { border-style: solid; border-color: var(--color-primary); color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 8%, transparent); }
.il-chip:disabled { cursor: not-allowed; opacity: .5; }
.il-count { font-family: var(--font-command); font-size: 10px; font-weight: 700; }
.il-state, .il-empty { padding: 24px; display: flex; flex-direction: column; align-items: center; gap: 8px; text-align: center; }
.il-empty { border: 1px dashed color-mix(in srgb, var(--color-primary) 20%, transparent); }
.il-state-title { margin: 0; font-family: var(--font-command); font-size: 12px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); }
.il-state-text { margin: 0; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.il-rows { list-style: none; margin: 0; padding: 0; overflow: hidden; }
.il-rows li { border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 10%, transparent); }
.il-rows li:last-child { border-bottom: 0; }
.il-row { display: flex; align-items: center; gap: 12px; width: 100%; padding: 10px 14px; background: transparent; border: 0; cursor: pointer; text-align: left; color: inherit; transition: background .15s; }
.il-row:hover { background: color-mix(in srgb, var(--color-primary) 3%, transparent); }
.il-row:focus-visible { outline: 2px solid var(--color-primary); outline-offset: -2px; }
.il-num { width: 120px; flex-shrink: 0; font-family: var(--font-terminal); font-size: 8px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--color-tertiary); overflow-wrap: anywhere; }
.il-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.il-client { font-family: var(--font-command); font-size: 11px; font-weight: 600; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.il-meta { font-family: var(--font-terminal); font-size: 8px; letter-spacing: .05em; text-transform: uppercase; color: var(--color-tertiary); margin-top: 2px; }
.il-money { display: flex; flex-direction: column; align-items: flex-end; flex-shrink: 0; }
.il-total { font-family: var(--font-command); font-size: 13px; font-weight: 700; letter-spacing: -.02em; color: var(--color-on-surface); }
.il-neg { color: var(--color-on-surface-variant); }
.il-badge { flex-shrink: 0; min-width: 64px; justify-content: center; }
.il-skel { display: flex; gap: 12px; padding: 14px; }
.il-skel-bar { display: block; height: 12px; background: var(--color-surface-container-high); animation: il-pulse 1.2s ease-in-out infinite; }
@keyframes il-pulse { 50% { opacity: .5; } }
@media (prefers-reduced-motion: reduce) { .il-skel-bar { animation: none; } }
.il-inline-error { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.il-retry { margin-left: 6px; background: none; border: 0; padding: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-primary); text-decoration: underline; }
@media (max-width: 768px) {
  .il-chip { height: 44px; padding: 0 12px; }
  .il-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: "num badge" "main money"; gap: 6px 10px; min-height: 56px; }
  .il-num { width: auto; grid-area: num; align-self: center; }
  .il-badge { grid-area: badge; justify-self: end; }
  .il-main { grid-area: main; }
  .il-money { grid-area: money; }
  .il-tap, .il-retry { min-height: 44px; }
}
</style>
