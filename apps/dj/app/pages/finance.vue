<script setup lang="ts">
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { TrendingUp, Download } from 'lucide-vue-next'
import { useInvoiceStore } from '../stores/invoice'
import InvoiceList from '../components/finance/InvoiceList.vue'
import InvoiceWorkspace from '../components/finance/InvoiceWorkspace.vue'

useHead({ title: 'Finance — KlubHub DJ' })

const isDevOrStaging = import.meta.env.DEV || import.meta.env.MODE === 'staging'

// Mock finance data (future phases will wire to API)
const stats = {
  mtd: '€3,400',
  pending: '€1,200',
  ytd: '€14,800',
  nextPayout: 'APR 01, 2026',
}

const barData = [
  { label: 'OCT', value: 60 },
  { label: 'NOV', value: 80 },
  { label: 'DEC', value: 45 },
  { label: 'JAN', value: 90 },
  { label: 'FEB', value: 55 },
  { label: 'MAR', value: 100 },
]

const transactions = [
  { id: 1, title: 'Club Alpha — Mainstage',           date: 'MAR 29', amount: '€1,200', type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 2, title: 'Club Beta — Resident Night',       date: 'MAR 8',  amount: '€800',  type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 3, title: 'Sample Records — Royalties Q1',    date: 'MAR 15', amount: '€340',  type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 4, title: 'Studio Session — Mixing Services', date: 'MAR 2',  amount: '€620',  type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 5, title: 'Travel & Accommodation',           date: 'MAR 27', amount: '-€180', type: 'expense', accentClass: 'accent-bar-published' },
  { id: 6, title: 'Gear: USB Drives',                 date: 'MAR 10', amount: '-€95',  type: 'expense', accentClass: 'accent-bar-published' },
]

const invoiceStore = useInvoiceStore()
const { disabled: invoiceDisabled } = storeToRefs(invoiceStore)

onMounted(() => {
  void invoiceStore.fetchInvoices()
})
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">
    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">FINANCE</div>
        <div class="page-sub">EARNINGS · INVOICES · TRANSACTIONS</div>
      </div>
      <button 
        class="btn-hud btn-hud-ghost" 
        style="padding:0 14px;"
        :disabled="true"
        title="Coming soon - Phase 5"
      >
        <Download style="width:12px;height:12px;" aria-hidden="true" />
        EXPORT CSV
      </button>
    </div>

    <!-- Scrollable body -->
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

      <!-- Production: honest empty state -->
      <div v-if="!isDevOrStaging" class="glass hud-card" style="padding:32px 24px;text-align:center;">
        <TrendingUp style="width:48px;height:48px;color:var(--color-tertiary);margin:0 auto 16px;" aria-hidden="true" />
        <div style="font-family:var(--font-command);font-size:18px;font-weight:600;color:var(--color-on-surface);margin-bottom:8px;">
          EARNINGS — NOT YET WIRED
        </div>
        <div style="font-family:var(--font-data);font-size:13px;color:var(--color-on-surface-variant);max-width:400px;margin:0 auto 24px;">
          Invoices and payments are live above. Earnings charts, transaction tracking and CSV export
          are not wired yet, so no data is shown here.
        </div>
        <div style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">
          This page will be replaced when the finance module ships.
        </div>
      </div>

      <!-- Dev/Staging: labelled mock preview -->
      <div v-else>
        <div class="glass accent-bar-ready" style="margin:0 20px 14px;padding:10px 16px;display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;border:1px dashed color-mix(in srgb, var(--color-primary) 40%, transparent);">
          <span class="font-terminal tracking-terminal text-primary" style="font-size:8px;letter-spacing:.06em;text-transform:uppercase;">
            ⚠ DEMO DATA — EARNINGS &amp; TRANSACTIONS NOT YET WIRED
          </span>
          <span class="font-terminal tracking-terminal text-primary quiet" style="font-size:8px;letter-spacing:.06em;text-transform:uppercase;">
            This preview will not appear in production
          </span>
        </div>

        <!-- Stats row -->
        <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:10px;" class="stats-grid">
          <div class="glass accent-bar-ready" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">EARNED — MAR</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-primary);letter-spacing:-.02em;text-shadow:0 0 30px rgba(150,248,255,.2);">{{ stats.mtd }}</div>
          </div>
          <div class="glass accent-bar-draft" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">PENDING</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-secondary);letter-spacing:-.02em;">{{ stats.pending }}</div>
            <div class="spost-meta" style="margin-top:3px;">
              <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              PAYOUT {{ stats.nextPayout }}
            </div>
          </div>
          <div class="glass" style="padding:12px 14px;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
              <div class="section-lbl">YTD TOTAL</div>
              <TrendingUp style="width:12px;height:12px;color:var(--color-tertiary);opacity:.5;" />
            </div>
            <div style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;">{{ stats.ytd }}</div>
          </div>
        </div>

        <!-- 2-col layout: chart + transactions -->
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:14px;" class="finance-grid">
          <!-- Monthly earnings chart -->
          <div>
            <div class="section-lbl" style="margin-bottom:8px;">MONTHLY EARNINGS <span class="font-terminal tracking-terminal text-primary" style="font-size:7px;letter-spacing:.04em;text-transform:uppercase;">(MOCK)</span></div>
            <div class="glass hud-card" style="padding:16px 14px;">
              <div class="bar-chart" style="margin-bottom:8px;">
                <div
                  v-for="bar in barData"
                  :key="bar.label"
                  class="bar-col"
                >
                  <div
                    class="bar"
                    :style="{ height: bar.value + '%' }"
                  />
                  <div class="bar-lbl">{{ bar.label }}</div>
                </div>
              </div>
              <div class="prog-track">
                <div class="prog-fill" style="width:68%;" />
              </div>
              <div class="spost-meta" style="margin-top:5px;">
                <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="var(--color-primary)" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>
                68% OF MONTHLY TARGET
              </div>
            </div>
          </div>

          <!-- Recent transactions -->
          <div>
            <div class="section-lbl" style="margin-bottom:8px;">RECENT TRANSACTIONS <span class="font-terminal tracking-terminal text-primary" style="font-size:7px;letter-spacing:.04em;text-transform:uppercase;">(MOCK)</span></div>
            <div class="glass" style="overflow:hidden;">
              <div
                v-for="tx in transactions"
                :key="tx.id"
                class="tx-row"
                :class="tx.accentClass"
              >
                <div class="tx-info">
                  <div class="tx-title">{{ tx.title }}</div>
                  <div class="tx-meta">{{ tx.date }}</div>
                </div>
                <div
                  class="tx-amount"
                  :style="tx.type === 'income' ? 'color:var(--color-primary)' : 'color:var(--color-error)'"
                >
                  {{ tx.amount }}
                </div>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
    <InvoiceWorkspace />
  </div>
</template>

<style scoped>
.new-invoice-btn:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; }
@media (max-width: 768px) {
  .new-invoice-btn { min-height: 44px; height: 44px; }
  .stats-grid  { grid-template-columns: 1fr !important; }
  .finance-grid { grid-template-columns: 1fr !important; }
}
</style>