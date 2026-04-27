<script setup lang="ts">
import { TrendingUp, Download } from 'lucide-vue-next'

useHead({ title: 'Finance — KlubHub DJ' })

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
  { id: 1, title: 'Cyber Void Berlin — Mainstage',   date: 'MAR 29', amount: '€1,200', type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 2, title: 'Tresor — Resident Night',          date: 'MAR 8',  amount: '€800',  type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 3, title: 'Ostgut Ton — Royalties Q1',        date: 'MAR 15', amount: '€340',  type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 4, title: 'Studio Session — Mixing Services', date: 'MAR 2',  amount: '€620',  type: 'income',  accentClass: 'accent-bar-ready' },
  { id: 5, title: 'Travel & Accommodation',           date: 'MAR 27', amount: '-€180', type: 'expense', accentClass: 'accent-bar-failed' },
  { id: 6, title: 'Gear: Pioneer CDJ USB Drives',    date: 'MAR 10', amount: '-€95',  type: 'expense', accentClass: 'accent-bar-failed' },
]

const invoices = [
  { num: 'INV-2026-012', client: 'CYBER VOID BERLIN', amount: '€1,200', status: 'paid',    badgeClass: 'badge-ready',    badgeLabel: 'PAID' },
  { num: 'INV-2026-011', client: 'TRESOR BERLIN',     amount: '€800',   status: 'paid',    badgeClass: 'badge-ready',    badgeLabel: 'PAID' },
  { num: 'INV-2026-010', client: 'FABRIC LONDON',     amount: '€1,500', status: 'pending', badgeClass: 'badge-draft',    badgeLabel: 'PENDING' },
  { num: 'INV-2026-009', client: 'BERGHAIN',          amount: '€2,000', status: 'pending', badgeClass: 'badge-draft',    badgeLabel: 'PENDING' },
  { num: 'INV-2026-008', client: 'DEKMANTEL FESTIVAL',amount: '€1,800', status: 'overdue', badgeClass: 'badge-failed',   badgeLabel: 'OVERDUE' },
]
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">FINANCE</div>
        <div class="page-sub">EARNINGS · INVOICES · TRANSACTIONS</div>
      </div>
      <button class="btn-hud btn-hud-ghost" style="padding:0 14px;">
        <Download style="width:12px;height:12px;" aria-hidden="true" />
        EXPORT CSV
      </button>
    </div>

    <!-- Scrollable body -->
    <div class="page-body">

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
            <TrendingUp style="width:12px;height:12px;color:var(--color-primary);opacity:.5;" />
          </div>
          <div style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;">{{ stats.ytd }}</div>
        </div>
      </div>

      <!-- 2-col layout: chart + transactions -->
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:14px;" class="finance-grid">

        <!-- Monthly earnings chart -->
        <div>
          <div class="section-lbl" style="margin-bottom:8px;">MONTHLY EARNINGS</div>
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
          <div class="section-lbl" style="margin-bottom:8px;">RECENT TRANSACTIONS</div>
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

      <!-- Invoices -->
      <div>
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
          <div class="section-lbl">INVOICES</div>
          <button class="btn-hud btn-hud-cta btn-hud-xs" style="padding:0 10px;">+ NEW INVOICE</button>
        </div>
        <div class="glass" style="overflow:hidden;">
          <div
            v-for="inv in invoices"
            :key="inv.num"
            style="display:flex;align-items:center;gap:10px;padding:11px 14px;border-bottom:1px solid rgba(46,46,49,.15);"
          >
            <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);width:80px;flex-shrink:0;">
              {{ inv.num }}
            </div>
            <div style="flex:1;font-family:var(--font-command);font-size:11px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;">
              {{ inv.client }}
            </div>
            <div style="font-family:var(--font-command);font-size:13px;font-weight:700;letter-spacing:-.02em;flex-shrink:0;color:var(--color-on-surface);">
              {{ inv.amount }}
            </div>
            <span class="badge-hud" :class="inv.badgeClass">{{ inv.badgeLabel }}</span>
          </div>
        </div>
      </div>

    </div>
  </div>
</template>

<style scoped>
@media (max-width: 768px) {
  .stats-grid  { grid-template-columns: 1fr !important; }
  .finance-grid { grid-template-columns: 1fr !important; }
}
</style>
