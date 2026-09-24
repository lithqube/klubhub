<script setup lang="ts">
import { TrendingUp } from 'lucide-vue-next'
import { useGigStore } from '../../stores/gig'
import { storeToRefs } from 'pinia'

const gigStore = useGigStore()
const { upcomingGigs, ytdEarned, avgFee } = storeToRefs(gigStore)

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
  <div class="stats-grid">
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
