<script setup lang="ts">
import { TrendingUp, AlertCircle, Loader } from 'lucide-vue-next'
import { useGigStore } from '../../stores/gig'
import { storeToRefs } from 'pinia'

const gigStore = useGigStore()
const { upcomingGigs, ytdEarned, avgFee, gigs, loading, loaded, error } = storeToRefs(gigStore)

function formatCurrency(value: number): string {
  if (value === 0) return '—'
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'EUR',
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(value)
}
</script>

<template>
  <div v-if="loading || !loaded && !error" class="glass stats-state" role="status"><Loader class="animate-spin" aria-hidden="true" /> <span class="section-lbl">LOADING GIG STATS...</span></div>
  <div v-else-if="error" class="glass stats-state stats-error" role="alert"><AlertCircle aria-hidden="true" /> <span>{{ error }}</span></div>
  <div v-else-if="gigs.length === 0" class="glass stats-state" role="status"><span class="section-lbl">NO GIG STATS YET</span></div>
  <div v-else class="stats-grid">
    <div class="glass accent-bar-ready" style="padding:12px 14px;">
      <div class="section-lbl" style="margin-bottom:4px;">UPCOMING</div>
      <div
        style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-primary);letter-spacing:-.02em;"
      >
        {{ upcomingGigs.length }}
      </div>
    </div>
    <div class="glass" style="padding:12px 14px;">
      <div class="section-lbl" style="margin-bottom:4px;">AVG FEE</div>
      <div
        style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;"
      >
        {{ formatCurrency(avgFee) }}
      </div>
    </div>
    <div class="glass" style="padding:12px 14px;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
        <div class="section-lbl">YTD EARNED</div>
        <TrendingUp
          style="width:12px;height:12px;color:var(--color-tertiary);opacity:.5;"
        />
      </div>
      <div
        style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;"
      >
        {{ formatCurrency(ytdEarned) }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.stats-state { padding: 16px; display: flex; align-items: center; justify-content: center; gap: 8px; margin-bottom: 12px; }
.stats-state svg { width: 13px; height: 13px; color: var(--color-primary); }
.stats-error { background: color-mix(in srgb, var(--color-error) 5%, var(--color-surface-container)); border: 1px solid color-mix(in srgb, var(--color-error) 15%, transparent); font-family: var(--font-data); font-size: 12px; color: var(--color-error); }
.stats-error svg { color: var(--color-error); }
.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
}
</style>
