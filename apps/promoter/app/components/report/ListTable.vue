<script setup lang="ts">
import type { ReportListRow } from '~/types/report'
import { LIST_TYPES } from '~/utils/guests'
import { formatPercent, sortLists } from '~/utils/report'

/** Arrivals by guest list (P2.4); artist lists first. */
const props = defineProps<{ rows: ReportListRow[] }>()
const rows = computed(() => sortLists(props.rows))
const typeLabel = (t: string) => LIST_TYPES.find(x => x.id === t)?.label ?? t.toUpperCase()
</script>

<template>
  <section class="glass block" aria-labelledby="report-lists-h">
    <h2 id="report-lists-h" class="section-lbl" style="margin:0 0 6px;">BY LIST</h2>
    <p v-if="!rows.length" style="margin:0;font-size:13px;color:var(--color-on-surface-variant);">This event has no guest lists.</p>
    <table v-else class="report-table stack" data-testid="report-lists">
      <thead>
        <tr>
          <th scope="col">LIST</th>
          <th scope="col" class="num">ARRIVED / GOING</th>
          <th scope="col" class="num">NO-SHOW</th>
          <th scope="col" class="num">HEADS IN / EXPECTED</th>
          <th scope="col" class="num">+1s USED</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.list_id">
          <td class="head">
            {{ r.name }} <span class="data-frag" style="margin-left:4px;">{{ typeLabel(r.type) }}</span>
          </td>
          <td class="num" data-label="ARRIVED / GOING">{{ r.arrived }} / {{ r.going }}</td>
          <td class="num" data-label="NO-SHOW">{{ formatPercent(r.no_show_rate) }}</td>
          <td class="num" data-label="HEADS IN / EXPECTED">{{ r.heads_admitted }} / {{ r.heads_expected }}</td>
          <td class="num" data-label="+1s USED">{{ r.plus_ones_used }} / {{ r.plus_ones_allowed }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.block { padding: 12px; min-width: 0; }
</style>
