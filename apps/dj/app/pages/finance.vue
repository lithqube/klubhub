<script setup lang="ts">
// Finance page (FIN-01..FIN-10) — replaces the production empty state and
// the dev mock preview with the real earnings UI. Invoices (Phase 4) stay
// exactly where they were. The new finance store drives the entries list,
// monthly/yearly summary, P&L panes, and reconciliation prompts.
import { onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import InvoiceList from '../components/finance/InvoiceList.vue'
import InvoiceWorkspace from '../components/finance/InvoiceWorkspace.vue'
import EarningsWorkspace from '../components/finance/EarningsWorkspace.vue'
import EntryList from '../components/finance/EntryList.vue'
import EntrySummary from '../components/finance/EntrySummary.vue'
import EntryProfitLoss from '../components/finance/EntryProfitLoss.vue'
import EntryReconciliationDialog from '../components/finance/EntryReconciliationDialog.vue'
import EntryReconciliationPanel from '../components/finance/EntryReconciliationPanel.vue'
import { useInvoiceStore } from '../stores/invoice'
import { useEarningsStore } from '../stores/earnings'
import type { Entry, EntryReconciliation } from '../types/finance'

useHead({ title: 'Finance — KlubHub DJ' })

const invoiceStore = useInvoiceStore()
const earningsStore = useEarningsStore()
const { disabled: invoiceDisabled } = storeToRefs(invoiceStore)
const { listLoaded: entriesLoaded } = storeToRefs(earningsStore)

const voidingEntry = ref<Entry | null>(null)
const deletingEntry = ref<Entry | null>(null)
const reconDialogOpen = ref(false)
const reconDialogGigId = ref<string | null>(null)

function onEdit(entry: Entry): void {
  earningsStore.openEdit(entry.id)
}
function onVoid(entry: Entry): void {
  voidingEntry.value = entry
}
function onDelete(entry: Entry): void {
  deletingEntry.value = entry
}

async function confirmVoid(): Promise<void> {
  if (!voidingEntry.value) return
  const e = voidingEntry.value
  voidingEntry.value = null
  try {
    await earningsStore.voidEntry(e.id)
  } catch {
    // Already surfaced via the store's banner / inline error.
  }
}
async function confirmDelete(): Promise<void> {
  if (!deletingEntry.value) return
  const e = deletingEntry.value
  deletingEntry.value = null
  try {
    await earningsStore.deleteEntry(e.id)
  } catch {
    // Same.
  }
}

function onReconciliationResolved(_rec: EntryReconciliation | { id: string; resolution: string }): void {
  // Refresh summary + P&L so the page reflects the resolution immediately.
  void earningsStore.refreshSummary()
  void earningsStore.refreshProfitLoss()
  reconDialogOpen.value = false
}
function onReconciliationDismissed(): void {
  reconDialogOpen.value = false
}
function openReconciliationPanel(rec: EntryReconciliation): void {
  reconDialogGigId.value = rec.gig_id
  reconDialogOpen.value = true
}

onMounted(() => {
  void invoiceStore.fetchInvoices()
  void earningsStore.fetchEntries()
  void earningsStore.refreshSummary()
  void earningsStore.refreshProfitLoss()
})

watch(entriesLoaded, (loaded) => {
  if (!loaded) return
  // Re-fetch summary/P&L once we know which gigs have data.
  void earningsStore.refreshSummary()
  void earningsStore.refreshProfitLoss()
})
</script>

<template>
  <div class="finance-page" style="flex:1;display:flex;flex-direction:column;overflow:hidden;">
    <div class="page-header">
      <div>
        <div class="page-title">FINANCE</div>
        <div class="page-sub">EARNINGS · INVOICES · RECONCILIATIONS</div>
      </div>
    </div>

    <div class="page-body">
      <!-- Invoices: real data in every mode (Go API, or the in-memory mocks in dev) -->
      <section aria-labelledby="finance-invoices-title">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;margin-bottom:8px;">
          <h2 id="finance-invoices-title" class="section-lbl" style="margin:0;font-weight:600;">INVOICES</h2>
          <button
            type="button"
            class="btn-hud btn-hud-cta btn-hud-xs new-invoice-btn"
            style="padding:0 10px;"
            :disabled="invoiceDisabled"
            @click="invoiceStore.openCreate()"
          >
            + NEW INVOICE
          </button>
        </div>
        <InvoiceList />
      </section>

      <!-- Earnings reconciliation prompts (FIN-04 / FIN-05). One dialog shared by every prompt. -->
      <EntryReconciliationPanel @open="openReconciliationPanel" />

      <!-- Earnings entries -->
      <section aria-labelledby="finance-entries-title" class="finance-section">
        <div class="finance-section-head">
          <h2 id="finance-entries-title" class="section-lbl" style="margin:0;font-weight:600;">ENTRIES</h2>
          <div class="finance-cta-group">
            <button
              type="button"
              class="btn-hud btn-hud-cta btn-hud-xs"
              @click="earningsStore.openCreate('income')"
            >+ NEW INCOME</button>
            <button
              type="button"
              class="btn-hud btn-hud-cta btn-hud-xs"
              @click="earningsStore.openCreate('expense')"
            >+ NEW EXPENSE</button>
          </div>
        </div>
        <EntryList @edit="onEdit" @void="onVoid" @delete="onDelete" />
      </section>

      <!-- Monthly / yearly summary (FIN-06) and profit / loss (FIN-07) panes. -->
      <div class="finance-grid">
        <section class="finance-cell" aria-labelledby="finance-summary-title">
          <h2 id="finance-summary-title" class="section-lbl finance-cell-title">SUMMARY · FIN-06</h2>
          <EntrySummary />
        </section>
        <section class="finance-cell" aria-labelledby="finance-pl-title">
          <h2 id="finance-pl-title" class="section-lbl finance-cell-title">PROFIT &amp; LOSS · FIN-07</h2>
          <EntryProfitLoss />
        </section>
      </div>

      <p class="finance-notice" role="note">
        FIN-10 — No tax or VAT is computed anywhere on this page. Multi-currency totals are
        grouped by code and never converted (FIN-09). Auto-generated income tied to a paid gig
        (FIN-03) is read-only; void it instead of editing it.
      </p>
    </div>

    <InvoiceWorkspace />
    <EarningsWorkspace />

    <!-- Void confirm — manual income/expense only. -->
    <Teleport to="body">
      <div
        v-if="voidingEntry"
        role="dialog"
        aria-modal="true"
        class="finance-confirm-overlay"
        @click.self="voidingEntry = null"
      >
        <div class="glass finance-confirm">
          <h3 class="finance-confirm-title">VOID THIS ENTRY?</h3>
          <p class="finance-confirm-body">
            Voiding keeps the row on the books but zeroes it out of totals. You can edit the
            amount instead if you'd rather.
          </p>
          <div class="finance-confirm-actions">
            <button type="button" class="btn-hud" @click="voidingEntry = null">CANCEL</button>
            <button type="button" class="btn-hud btn-hud-violet" @click="confirmVoid">CONFIRM VOID</button>
          </div>
        </div>
      </div>
    </Teleport>
    <Teleport to="body">
      <div
        v-if="deletingEntry"
        role="dialog"
        aria-modal="true"
        class="finance-confirm-overlay"
        @click.self="deletingEntry = null"
      >
        <div class="glass finance-confirm">
          <h3 class="finance-confirm-title">DELETE THIS ENTRY?</h3>
          <p class="finance-confirm-body">This permanently removes the entry. Auto-generated entries cannot be deleted.</p>
          <div class="finance-confirm-actions">
            <button type="button" class="btn-hud" @click="deletingEntry = null">CANCEL</button>
            <button type="button" class="btn-hud btn-hud-error" @click="confirmDelete">DELETE</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Reconciliation dialog — mounted once; opens for any pending gig. -->
    <EntryReconciliationDialog
      v-model:open="reconDialogOpen"
      :gig-id="reconDialogGigId"
      @resolved="onReconciliationResolved"
      @dismissed="onReconciliationDismissed"
    />
  </div>
</template>

<style scoped>
.finance-page { gap: 12px; }
.finance-section { display: flex; flex-direction: column; gap: 8px; margin-top: 8px; }
.finance-section-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.finance-cta-group { display: inline-flex; gap: 6px; }
.finance-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 16px; }
.finance-cell { display: flex; flex-direction: column; gap: 6px; }
.finance-cell-title { margin: 0; font-weight: 600; }
.finance-notice { margin: 16px 0 0; padding: 10px 14px; font-family: var(--font-terminal); font-size: 9px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--color-tertiary); background: color-mix(in srgb, var(--color-tertiary) 8%, transparent); border-left: 3px solid var(--color-tertiary); }
.finance-confirm-overlay { position: fixed; inset: 0; z-index: 250; display: flex; align-items: center; justify-content: center; background: rgba(0, 0, 0, 0.85); }
.finance-confirm { padding: 24px; max-width: 360px; text-align: center; background: var(--color-surface-container); display: flex; flex-direction: column; gap: 12px; }
.finance-confirm-title { font-family: var(--font-command); font-size: 13px; font-weight: 700; margin: 0; letter-spacing: -.02em; text-transform: uppercase; }
.finance-confirm-body { margin: 0; font-size: 12px; color: var(--color-on-surface-variant); }
.finance-confirm-actions { display: flex; justify-content: center; gap: 8px; }
.new-invoice-btn:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; }
@media (max-width: 768px) {
  .finance-grid { grid-template-columns: 1fr !important; }
  .finance-section-head { flex-direction: column; align-items: stretch; }
  .finance-cta-group { justify-content: flex-end; }
  .finance-cta-group .btn-hud { min-height: 44px; }
}
</style>
