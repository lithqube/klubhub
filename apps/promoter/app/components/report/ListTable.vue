<script setup lang="ts">
import type { ReportListRow } from '~/types/report'
import { LIST_TYPES } from '~/utils/guests'
import { countPair, formatPercent, sortLists } from '~/utils/report'

/**
 * Arrivals by guest list (P2.4); artist lists first. Explicit table roles
 * keep the semantics when phones restyle each row as a card.
 */
const props = defineProps<{ rows: ReportListRow[] }>()
const rows = computed(() => sortLists(props.rows))
const typeLabel = (t: string) => LIST_TYPES.find(x => x.id === t)?.label ?? t.toUpperCase()
</script>

<template>
  <section class="glass block" aria-labelledby="report-lists-h">
    <h2 id="report-lists-h" class="section-lbl" style="margin:0 0 6px;">BY LIST</h2>
    <p v-if="!rows.length" style="margin:0;font-size:13px;color:var(--color-on-surface-variant);">This event has no guest lists.</p>
    <div v-else class="report-table-wrap">
      <table role="table" class="report-table stack" data-testid="report-lists">
        <thead role="rowgroup">
          <tr role="row">
            <th role="columnheader" scope="col">LIST</th>
            <th role="columnheader" scope="col" class="num">ARRIVED / GOING</th>
            <th role="columnheader" scope="col" class="num">NO-SHOW</th>
            <th role="columnheader" scope="col" class="num">HEADS CHECKED IN / EXPECTED</th>
            <th role="columnheader" scope="col" class="num">+1s USED / ALLOWED</th>
          </tr>
        </thead>
        <tbody role="rowgroup">
          <tr v-for="r in rows" :key="r.list_id" role="row">
            <td role="cell" class="head">
              {{ r.name }} <span class="data-frag" style="margin-left:4px;">{{ typeLabel(r.type) }}</span>
            </td>
            <td role="cell" class="num" data-label="ARRIVED / GOING">{{ countPair(r.arrived, r.going, 'going') }}</td>
            <td role="cell" class="num" data-label="NO-SHOW">{{ formatPercent(r.no_show_rate) }}</td>
            <td role="cell" class="num" data-label="HEADS / EXPECTED">{{ countPair(r.heads_admitted, r.heads_expected, 'expected') }}</td>
            <td role="cell" class="num" data-label="+1s / ALLOWED">{{ r.plus_ones_used }} / {{ r.plus_ones_allowed }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.block { padding: 12px; min-width: 0; }
</style>
