<script setup lang="ts">
import { TriangleAlert } from 'lucide-vue-next'
import type { CurvePoint, ReportTotals } from '~/types/report'
import { kpiTiles } from '~/utils/report'

/**
 * The report's headline numbers (P2.4). THROUGH THE DOOR leads (the only
 * total that includes walk-ups and manual counts); each tile then counts one
 * thing: label, one value, one sentence of context. Counts only, never names.
 */
const props = defineProps<{ totals: ReportTotals, curve: CurvePoint[], capacity: number | null, tz: string, eventId: string }>()

const tiles = computed(() => kpiTiles(props.totals, props.curve, props.capacity, props.tz))
</script>

<template>
  <section aria-labelledby="report-kpi-h">
    <h2 id="report-kpi-h" class="sr-only">Key numbers</h2>
    <dl class="kpis">
      <div v-for="k in tiles" :key="k.id" class="kpi glass" :class="{ hero: k.hero }" :data-testid="`kpi-${k.id}`">
        <dt class="section-lbl">{{ k.label }}</dt>
        <dd class="value">
          {{ k.value }}<span v-if="k.of !== null" class="of"> / {{ k.of }}</span>
        </dd>
        <dd class="sub">{{ k.sub }}</dd>
      </div>
    </dl>
    <details class="how" data-testid="kpi-definitions">
      <summary class="section-lbl">HOW THESE ARE COUNTED</summary>
      <dl>
        <dt>THROUGH THE DOOR</dt>
        <dd>Every head that came in: list guests and tickets checked in (re-entry counts again), manual counts and walk-ups.</dd>
        <dt>ARRIVED</dt>
        <dd>A guest arrived when the door checked them in at least once. Guests who were pending or declined but let in anyway count too.</dd>
        <dt>NO-SHOW</dt>
        <dd>Guests marked going who never arrived, out of all going guests.</dd>
        <dt>LIST &amp; TICKET HEADS</dt>
        <dd>Heads checked in from guest lists and scanned tickets, including re-entry. Walk-ups and manual counts are not in it.</dd>
        <dt>+1s USED</dt>
        <dd>Extra heads a guest brought, never more than the +1s they were allowed.</dd>
        <dt>WALK-UPS</dt>
        <dd>People with no list spot or ticket who paid at the door.</dd>
        <dt>PEAK INSIDE</dt>
        <dd>People inside at the end of the busiest 15 minutes: everyone in minus everyone who left.</dd>
      </dl>
    </details>
    <p v-if="totals.conflicts" class="glass conflicts" data-testid="kpi-conflicts">
      <TriangleAlert style="width:14px;height:14px;flex:none;color:var(--color-status-archived);" aria-hidden="true" />
      <span>
        <strong>{{ totals.conflicts }}</strong> {{ totals.conflicts === 1 ? 'check-in was' : 'check-ins were' }} flagged as a conflict (more heads than allowed across doors).
        <NuxtLink :to="`/events/${eventId}/door`" class="link">Review them on the DOOR tab</NuxtLink>.
      </span>
    </p>
  </section>
</template>

<style scoped>
.kpis {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
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
.kpi.hero {
  grid-column: span 2;
  border-left: 3px solid var(--color-primary);
}
.kpi dd { margin: 0; }
.value {
  font-family: var(--font-command);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.1;
  color: var(--color-on-surface);
  font-variant-numeric: tabular-nums;
}
.hero .value { font-size: 40px; }
.of {
  font-size: 15px;
  font-weight: 500;
  color: var(--color-on-surface-variant);
}
.sub {
  font-family: var(--font-data);
  font-size: 11px;
  line-height: 1.35;
  color: var(--color-on-surface-variant);
}
.how {
  margin-top: 8px;
  font-family: var(--font-data);
  font-size: 12px;
  color: var(--color-on-surface-variant);
}
.how summary {
  cursor: pointer;
  min-height: 44px;
  display: flex;
  align-items: center;
  font-size: 10px;
}
.how summary { list-style: none; gap: 6px; }
.how summary::-webkit-details-marker { display: none; }
.how summary::before {
  content: '';
  width: 6px;
  height: 6px;
  margin: 0 2px;
  border-right: 1.5px solid currentColor;
  border-bottom: 1.5px solid currentColor;
  transform: rotate(-45deg);
  transition: transform .15s;
}
.how[open] summary::before { transform: rotate(45deg); }
@media (prefers-reduced-motion: reduce) { .how summary::before { transition: none; } }
.how summary:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 2px; }
.how dl {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 6px 12px;
  margin: 0 0 4px;
}
.how dt {
  font-family: var(--font-terminal);
  font-size: 10px;
  letter-spacing: .06em;
  color: var(--color-tertiary);
  padding-top: 2px;
}
.how dd { margin: 0; max-width: 70ch; }
.conflicts {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0 0;
  padding: 8px 12px;
  font-size: 13px;
  border-left: 3px solid var(--color-status-archived);
}
.link { color: var(--color-primary); text-decoration: underline; }
@media (max-width: 480px) {
  .how dl { grid-template-columns: minmax(0, 1fr); gap: 2px; }
  .how dd { margin-bottom: 6px; }
}
@media (max-width: 380px) {
  .kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .value { font-size: 22px; }
  .hero .value { font-size: 32px; }
}
</style>
