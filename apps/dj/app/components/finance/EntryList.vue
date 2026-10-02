<script setup lang="ts">
// Filterable list of finance entries (income / expense). The entries store
// is the source of truth; this component never fetches. Two sources of
// "auto-generated" income are tagged differently from manual rows. Status
// chip switches the list between active, voided, and so on. The income /
// expense kind toggle reuses the same entries list.
//
// Pure presentation: emits openEdit(id) and voidEntry(id) for the page to
// dispatch through the store.
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import type { Entry, EntryKind, EntryStatus } from '../../types/finance'
import { useEarningsStore } from '../../stores/earnings'
import { formatMinor } from '../../utils/money'

const emit = defineEmits<{
  edit: [entry: Entry]
  void: [entry: Entry]
  delete: [entry: Entry]
}>()

const store = useEarningsStore()
const { entries, filteredEntries, filterCounts, filter, listLoading, listError, disabled, listLoaded } = storeToRefs(store)

const KIND_OPTIONS: EntryKind[] = ['income', 'expense']
const STATUS_OPTIONS: EntryStatus[] = ['active', 'voided']

const state = computed<'disabled' | 'loading' | 'error' | 'empty' | 'rows'>(() => {
  if (disabled.value) return 'disabled'
  if (listError.value && !listLoaded.value) return 'error'
  if (!listLoaded.value || listLoading.value) return 'loading'
  if (!entries.value.length) return 'empty'
  if (!filteredEntries.value.length) return 'rows'
  return 'rows'
})

const stateRows = computed(() => filteredEntries.value)

function rowDateLabel(e: Entry): string {
  return e.entry_date
}

function rowAmountLabel(e: Entry): string {
  return `${e.kind === 'expense' ? '-' : ''}${formatMinor(e.amount_minor, e.currency)}`
}

function rowStatusLabel(e: Entry): string {
  if (e.status === 'voided') return 'voided'
  if (e.auto_generated) return e.source_kind === 'gig_payment' ? 'gig' : 'auto'
  return 'manual'
}

function rowAriaLabel(e: Entry): string {
  const kindWord = e.kind === 'income' ? 'income' : 'expense'
  const detail = isIncome(e)
    ? (e.description || e.notes || 'no description')
    : (e.notes || e.description || 'no notes')
  return [
    `${kindWord} ${formatMinor(e.amount_minor, e.currency)} on ${rowDateLabel(e)}`,
    detail,
    e.currency,
    rowStatusLabel(e),
    e.status === 'voided' ? 'voided' : '',
  ].filter(Boolean).join(', ')
}

function isIncome(e: Entry): boolean { return e.kind === 'income' }

function canEdit(e: Entry): boolean {
  return !e.auto_generated && e.status === 'active'
}

function canVoid(e: Entry): boolean {
  return e.status === 'active'
}

function canDelete(e: Entry): boolean {
  return !e.auto_generated && e.status === 'active'
}

// setFilter only updates filter (what the controls show as selected);
// filteredEntries reads filterSnapshot, which only fetchEntries advances —
// so every control here must also refetch, or the chips/selects change
// while the rows underneath stay on the old filter.
function applyFilters(): void {
  void store.fetchEntries(store.filterAsEntryFilter())
}
function setKind(k: '' | EntryKind): void {
  store.setFilter('kind', k as typeof filter.value.kind)
  applyFilters()
}
function setStatus(s: '' | EntryStatus): void {
  store.setFilter('status', s as typeof filter.value.status)
  applyFilters()
}
function setCurrency(c: string): void {
  store.setFilter('currency', c)
  applyFilters()
}
function clearFilters(): void {
  store.clearFilter()
  applyFilters()
}
</script>

<template>
  <div class="el">
    <div class="el-toolbar">
      <div class="el-chips" role="group" aria-label="Filter by kind">
        <button
          type="button"
          class="el-chip"
          :class="{ 'el-chip-on': filter.kind === '' }"
          :aria-pressed="filter.kind === ''"
          @click="setKind('')"
        >
          ALL<span class="el-count">{{ filterCounts.all }}</span>
        </button>
        <button
          v-for="k in KIND_OPTIONS"
          :key="k"
          type="button"
          class="el-chip"
          :class="{ 'el-chip-on': filter.kind === k }"
          :aria-pressed="filter.kind === k"
          @click="setKind(k)"
        >
          {{ k.toUpperCase() }}<span class="el-count">{{ k === 'income' ? filterCounts.income : filterCounts.expense }}</span>
        </button>
      </div>

      <div class="el-tools">
        <label class="el-field">
          <span class="el-field-lbl">CURRENCY</span>
          <select class="hud-input el-select" :value="filter.currency" @change="setCurrency(($event.target as HTMLSelectElement).value)">
            <option value="">All</option>
            <option v-for="c in store.currencies" :key="c" :value="c">{{ c }}</option>
          </select>
        </label>
        <label class="el-field">
          <span class="el-field-lbl">STATUS</span>
          <select class="hud-input el-select" :value="filter.status" @change="setStatus((($event.target as HTMLSelectElement).value as '' | EntryStatus))">
            <option value="">All</option>
            <option v-for="s in STATUS_OPTIONS" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <button v-if="filter.kind || filter.currency || filter.category || filter.status" type="button" class="btn-hud btn-hud-ghost btn-hud-xs" @click="clearFilters">RESET</button>
      </div>
    </div>

    <p class="el-help">FIN-10 — Earnings entries do not compute tax or VAT. Entries appear in their own currency; totals below are grouped by code and never converted.</p>

    <div v-if="state === 'disabled'" class="glass el-state" role="status">
      <p class="el-state-title">EARNINGS UNAVAILABLE</p>
      <p class="el-state-text">Finance isn't enabled on this server.</p>
    </div>

    <div v-else-if="state === 'loading'" class="glass" aria-busy="true" aria-label="Loading entries">
      <div v-for="n in 4" :key="n" class="el-skel">
        <span class="el-skel-bar" style="width:80px;" />
        <span class="el-skel-bar" style="flex:1;" />
        <span class="el-skel-bar" style="width:70px;" />
      </div>
    </div>

    <div v-else-if="state === 'error'" class="glass el-state" role="alert">
      <p class="el-state-title">COULD NOT LOAD ENTRIES</p>
      <p class="el-state-text">{{ listError?.message }}</p>
      <button type="button" class="btn-hud btn-hud-ghost el-tap" @click="store.fetchEntries(store.filterSnapshot)">RETRY</button>
    </div>

    <div v-else-if="state === 'empty'" class="glass hud-card el-empty">
      <p class="el-state-title">NO ENTRIES YET</p>
      <p class="el-state-text">Log your first income or expense above.</p>
    </div>

    <ul v-else-if="stateRows.length" class="glass el-rows" aria-label="Finance entries">
      <li v-for="e in stateRows" :key="e.id" class="el-row" :class="{ 'el-row-void': e.status === 'voided', 'el-row-auto': e.auto_generated }">
        <div class="el-row-main">
          <span class="el-row-date">{{ rowDateLabel(e) }}</span>
          <div class="el-row-text">
            <span class="el-row-title">{{ isIncome(e) ? (e.description || e.notes || '—') : (e.notes || e.description || '—') }}</span>
            <span class="el-row-meta">
              {{ e.category.toUpperCase() }}
              <template v-if="e.gig_id"> · GIG</template>
              <template v-if="e.source_kind !== 'manual'"> · {{ e.source_kind.replace('_', ' ').toUpperCase() }}</template>
              <template v-if="e.status === 'voided'"> · VOIDED</template>
            </span>
          </div>
          <span
            class="el-row-amount"
            :class="{ 'el-row-neg': e.kind === 'expense', 'el-row-voided-amt': e.status === 'voided' }"
          >{{ rowAmountLabel(e) }}</span>
        </div>
        <div class="el-row-actions">
          <button
            v-if="canEdit(e)"
            type="button"
            class="btn-hud btn-hud-ghost btn-hud-xs"
            :aria-label="`Edit ${rowAriaLabel(e)}`"
            @click="emit('edit', e)"
          >
            EDIT
          </button>
          <button
            v-if="canVoid(e)"
            type="button"
            class="btn-hud btn-hud-ghost btn-hud-xs"
            :aria-label="`Void ${rowAriaLabel(e)}`"
            @click="emit('void', e)"
          >
            VOID
          </button>
          <button
            v-if="canDelete(e)"
            type="button"
            class="btn-hud btn-hud-ghost btn-hud-xs el-danger"
            :aria-label="`Delete ${rowAriaLabel(e)}`"
            @click="emit('delete', e)"
          >
            DELETE
          </button>
        </div>
      </li>
    </ul>
    <div v-else class="glass el-state" role="status">
      <p class="el-state-text">No entries match the current filter.</p>
      <button type="button" class="btn-hud btn-hud-ghost el-tap" @click="clearFilters">RESET FILTERS</button>
    </div>

    <p v-if="listError && listLoaded" class="el-inline-error" role="alert">
      Could not refresh: {{ listError.message }}
      <button type="button" class="el-retry" @click="store.fetchEntries(store.filterSnapshot)">RETRY</button>
    </p>
  </div>
</template>

<style scoped>
.el { display: flex; flex-direction: column; gap: 8px; }
.el-toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 12px; }
.el-chips { display: inline-flex; flex-wrap: wrap; gap: 4px; }
.el-chip { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; cursor: pointer; background: transparent; border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); transition: color .15s, border-color .15s; }
.el-chip:hover { color: var(--color-primary); }
.el-chip:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 1px; }
.el-chip-on { border-style: solid; border-color: var(--color-primary); color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 8%, transparent); }
.el-count { font-family: var(--font-command); font-size: 10px; font-weight: 700; }
.el-tools { display: inline-flex; align-items: end; gap: 8px; flex-wrap: wrap; }
.el-field { display: flex; flex-direction: column; gap: 4px; }
.el-field-lbl { font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-on-surface-variant); text-transform: uppercase; }
.el-select { min-width: 110px; height: 28px; padding: 0 8px; font-family: var(--font-command); font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: .04em; }
.el-help { margin: 0; font-family: var(--font-data); font-size: 11px; color: var(--color-on-surface-variant); }
.el-state, .el-empty { padding: 24px; display: flex; flex-direction: column; align-items: center; gap: 8px; text-align: center; }
.el-empty { border: 1px dashed color-mix(in srgb, var(--color-primary) 20%, transparent); }
.el-state-title { margin: 0; font-family: var(--font-command); font-size: 12px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); }
.el-state-text { margin: 0; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.el-rows { list-style: none; margin: 0; padding: 0; overflow: hidden; }
.el-row { display: flex; flex-direction: column; gap: 8px; padding: 10px 14px; border-bottom: 1px dashed color-mix(in srgb, var(--color-on-surface) 10%, transparent); }
.el-row:last-child { border-bottom: 0; }
.el-row-void { opacity: 0.65; }
.el-row-auto { background: color-mix(in srgb, var(--color-primary) 4%, transparent); }
.el-row-main { display: grid; grid-template-columns: 110px 1fr auto; align-items: center; gap: 10px; }
.el-row-date { font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--color-tertiary); }
.el-row-text { display: flex; flex-direction: column; min-width: 0; }
.el-row-title { font-family: var(--font-command); font-size: 12px; font-weight: 600; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.el-row-meta { margin-top: 2px; font-family: var(--font-terminal); font-size: 8px; letter-spacing: .05em; text-transform: uppercase; color: var(--color-tertiary); }
.el-row-amount { font-family: var(--font-command); font-size: 14px; font-weight: 700; color: var(--color-primary); text-align: right; }
.el-row-neg { color: var(--color-error); }
.el-row-voided-amt { text-decoration: line-through; }
.el-row-actions { display: flex; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
.el-danger { color: var(--color-error); }
.el-danger:hover { background: color-mix(in srgb, var(--color-error) 8%, transparent); }
.el-skel { display: flex; gap: 12px; padding: 14px; }
.el-skel-bar { display: block; height: 12px; background: var(--color-surface-container-high); animation: el-pulse 1.2s ease-in-out infinite; }
@keyframes el-pulse { 50% { opacity: .5; } }
.el-inline-error { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.el-retry { margin-left: 6px; background: none; border: 0; padding: 0; cursor: pointer; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; color: var(--color-primary); text-decoration: underline; }
@media (max-width: 768px) {
  .el-row-main { grid-template-columns: auto 1fr; row-gap: 4px; }
  .el-row-amount { grid-column: 2; text-align: left; }
  .el-row-date { grid-row: 2; }
  .el-row-text { grid-column: 2; }
  .el-tools { align-items: stretch; }
  .el-chip { min-height: 44px; }
}
</style>
