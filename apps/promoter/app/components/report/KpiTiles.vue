<script setup lang="ts">
import { TriangleAlert } from 'lucide-vue-next'
import type { ReportTotals } from '~/types/report'
import { timeLabel } from '~/utils/datetime'
import { formatCount, formatPercent } from '~/utils/report'

/**
 * The report's headline numbers (P2.4). Each tile: label, one value, one
 * line of context. Counts only, never names.
 */
const props = defineProps<{ totals: ReportTotals, capacity: number | null, tz: string }>()

const tiles = computed(() => {
  const t = props.totals
  return [
    { id: 'arrived', label: 'ARRIVED', value: formatCount(t.guests_arrived), of: formatCount(t.guests_going), sub: 'guests arrived · going' },
    { id: 'no-show', label: 'NO-SHOW', value: formatPercent(t.no_show_rate), of: null, sub: t.guests_going ? `of ${formatCount(t.guests_going)} going guests` : 'nobody was going' },
    {
      id: 'heads', label: 'HEADS IN', value: formatCount(t.heads_admitted), of: formatCount(t.heads_expected),
      sub: `admitted · expected${props.capacity ? ` · cap. ${formatCount(props.capacity)}` : ''}`,
    },
    { id: 'plus-ones', label: '+1s USED', value: formatCount(t.plus_ones_used), of: formatCount(t.plus_ones_allowed), sub: 'used · allowed' },
    { id: 'tickets', label: 'TICKETS SCANNED', value: formatCount(t.tickets_scanned), of: formatCount(t.tickets_valid), sub: 'scanned · valid tickets' },
    { id: 'walkups', label: 'WALK-UPS', value: formatCount(t.walkups), of: null, sub: 'paid at the door' },
    {
      id: 'peak', label: 'PEAK INSIDE', value: formatCount(t.peak_occupancy), of: null,
      sub: t.peak_at ? `at ${timeLabel(t.peak_at, props.tz)}` : 'no one inside yet',
    },
  ]
})
</script>

<template>
  <section aria-labelledby="report-kpi-h">
    <h2 id="report-kpi-h" class="sr-only">Key numbers</h2>
    <dl class="kpis">
      <div v-for="k in tiles" :key="k.id" class="kpi glass" :data-testid="`kpi-${k.id}`">
        <dt class="section-lbl">{{ k.label }}</dt>
        <dd class="value">
          {{ k.value }}<span v-if="k.of !== null" class="of"> / {{ k.of }}</span>
        </dd>
        <dd class="sub">{{ k.sub }}</dd>
      </div>
    </dl>
    <p v-if="totals.conflicts" class="glass conflicts" data-testid="kpi-conflicts">
      <TriangleAlert style="width:14px;height:14px;flex:none;color:var(--color-status-archived);" aria-hidden="true" />
      <span><strong>{{ totals.conflicts }}</strong> {{ totals.conflicts === 1 ? 'check-in was' : 'check-ins were' }} flagged as a conflict (more heads than allowed across doors).</span>
    </p>
  </section>
</template>

<style scoped>
.kpis {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 8px;
  margin: 0;
}
.kpi {
  padding: 10px 12px;
  display: grid;
  gap: 4px;
  align-content: start;
  min-width: 0;
}
.kpi dd { margin: 0; }
.value {
  font-family: var(--font-command);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.1;
  color: var(--color-on-surface);
}
.of {
  font-size: 15px;
  font-weight: 500;
  color: var(--color-on-surface-variant);
}
.sub {
  font-family: var(--font-data);
  font-size: 11px;
  color: var(--color-on-surface-variant);
}
.conflicts {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0 0;
  padding: 8px 12px;
  font-size: 13px;
  border-left: 3px solid var(--color-status-archived);
}
@media (max-width: 380px) {
  .kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .value { font-size: 22px; }
}
</style>
