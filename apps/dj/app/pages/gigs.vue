<script setup lang="ts">
import { Plus, TrendingUp } from 'lucide-vue-next'

useHead({ title: 'Gigs — KlubHub DJ' })

// Mock gig data (future phases will wire to API)
const gigs = [
  {
    id: 1,
    mon: 'MAR',
    day: '29',
    venue: 'CYBER VOID',
    details: 'MAINSTAGE · BERLIN, DE',
    fee: '€1,200',
    status: 'confirmed',
    accentClass: 'accent-bar-ready',
    badgeClass: 'badge-ready',
    badgeLabel: 'CONFIRMED',
  },
  {
    id: 2,
    mon: 'APR',
    day: '12',
    venue: 'TRESOR',
    details: 'GLOBUS FLOOR · BERLIN, DE',
    fee: '€900',
    status: 'confirmed',
    accentClass: 'accent-bar-ready',
    badgeClass: 'badge-ready',
    badgeLabel: 'CONFIRMED',
  },
  {
    id: 3,
    mon: 'APR',
    day: '26',
    venue: 'FABRIC LONDON',
    details: 'ROOM 1 · LONDON, UK',
    fee: '€1,500',
    status: 'pending',
    accentClass: 'accent-bar-draft',
    badgeClass: 'badge-draft',
    badgeLabel: 'PENDING',
  },
  {
    id: 4,
    mon: 'MAY',
    day: '3',
    venue: 'BERGHAIN',
    details: 'MAIN FLOOR · BERLIN, DE',
    fee: '€2,000',
    status: 'pending',
    accentClass: 'accent-bar-draft',
    badgeClass: 'badge-draft',
    badgeLabel: 'PENDING',
  },
  {
    id: 5,
    mon: 'MAR',
    day: '8',
    venue: 'TRESOR RESIDENT NIGHT',
    details: 'GLOBUS · BERLIN, DE',
    fee: '€800',
    status: 'archived',
    accentClass: 'accent-bar-archived',
    badgeClass: 'badge-archived',
    badgeLabel: 'PAST',
  },
]

const stats = {
  upcoming: 4,
  totalYtd: '€14,800',
  avgFee: '€1,280',
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">GIGS</div>
        <div class="page-sub">GIG TRACKER · SCHEDULE & HISTORY</div>
      </div>
      <button class="btn-hud btn-hud-cta" style="padding:0 14px;">
        <Plus style="width:12px;height:12px;" aria-hidden="true" />
        ADD GIG
      </button>
    </div>

    <!-- Scrollable body -->
    <div class="page-body">

      <!-- Stats row -->
      <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:10px;" class="stats-grid">
        <div class="glass accent-bar-ready" style="padding:12px 14px;">
          <div class="section-lbl" style="margin-bottom:4px;">UPCOMING</div>
          <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-primary);letter-spacing:-.02em;">{{ stats.upcoming }}</div>
        </div>
        <div class="glass accent-bar-draft" style="padding:12px 14px;">
          <div class="section-lbl" style="margin-bottom:4px;">AVG FEE</div>
          <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-secondary);letter-spacing:-.02em;">{{ stats.avgFee }}</div>
        </div>
        <div class="glass" style="padding:12px 14px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
            <div class="section-lbl">YTD EARNED</div>
            <TrendingUp style="width:12px;height:12px;color:var(--color-primary);opacity:.5;" />
          </div>
          <div style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;">{{ stats.totalYtd }}</div>
        </div>
      </div>

      <!-- Gig list -->
      <div>
        <div class="section-lbl" style="margin-bottom:8px;">GIG SCHEDULE</div>
        <div class="glass" style="overflow:hidden;">
          <div
            v-for="gig in gigs"
            :key="gig.id"
            class="gig-list-row"
            :class="gig.accentClass"
          >
            <!-- Date block -->
            <div class="gig-date-block">
              <div class="gig-date-mon">{{ gig.mon }}</div>
              <div class="gig-date-day">{{ gig.day }}</div>
            </div>

            <!-- Info -->
            <div style="flex:1;min-width:0;">
              <div class="gig-venue">{{ gig.venue }}</div>
              <div class="gig-meta">
                <span class="section-lbl">{{ gig.details }}</span>
              </div>
            </div>

            <!-- Fee -->
            <div class="gig-fee" :style="gig.status === 'confirmed' ? 'color:var(--color-primary)' : gig.status === 'pending' ? 'color:var(--color-secondary)' : 'color:var(--color-tertiary)'">
              {{ gig.fee }}
            </div>

            <!-- Status badge -->
            <span class="badge-hud" :class="gig.badgeClass">{{ gig.badgeLabel }}</span>
          </div>
        </div>
      </div>

      <!-- Empty state placeholder -->
      <div class="glass" style="padding:32px 24px;text-align:center;border:1px dashed rgba(150,248,255,.15);">
        <div style="font-family:var(--font-command);font-size:12px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;margin-bottom:6px;">
          FULL GIG MANAGEMENT
        </div>
        <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">
          INVOICING · CONTRACTS · TECH RIDER DISPATCH — COMING NEXT PHASE
        </div>
      </div>

    </div>
  </div>
</template>

<style scoped>
@media (max-width: 768px) {
  .stats-grid { grid-template-columns: 1fr !important; }
}
</style>
