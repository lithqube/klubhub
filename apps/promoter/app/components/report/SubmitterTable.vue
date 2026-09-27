<script setup lang="ts">
import { Download } from 'lucide-vue-next'
import type { ReportSubmitterRow } from '~/types/report'
import { countPair, formatPercent, sortSubmitters } from '~/utils/report'

/**
 * Arrivals by allocation (P2.4), artist lists first. LIST BACK sends a
 * submitter who came of their guests: names, +N, arrival and first-in time;
 * never email or phone. Several list-backs may be preparing at once.
 */
const props = defineProps<{ rows: ReportSubmitterRow[], busy: ReadonlySet<string> }>()
const emit = defineEmits<{ listBack: [row: ReportSubmitterRow] }>()
const rows = computed(() => sortSubmitters(props.rows))

function buttonLabel(r: ReportSubmitterRow): string {
  if (props.busy.has(r.allocation_id)) return 'PREPARING…'
  return r.going === 0 ? 'NO GUESTS' : 'LIST BACK CSV'
}
</script>

<template>
  <section id="report-submitters" class="glass block" aria-labelledby="report-subs-h">
    <h2 id="report-subs-h" class="section-lbl" style="margin:0 0 6px;">BY SUBMITTER</h2>
    <p v-if="!rows.length" style="margin:0;font-size:13px;color:var(--color-on-surface-variant);">No allocations on this event.</p>
    <div v-else class="report-table-wrap">
      <table role="table" class="report-table stack" data-testid="report-submitters">
        <thead role="rowgroup">
          <tr role="row">
            <th role="columnheader" scope="col">SUBMITTER</th>
            <th role="columnheader" scope="col" class="num">QUOTA</th>
            <th role="columnheader" scope="col" class="num">ARRIVED / GOING</th>
            <th role="columnheader" scope="col" class="num">NO-SHOW</th>
            <th role="columnheader" scope="col" class="num">HEADS CHECKED IN</th>
            <th role="columnheader" scope="col"><span class="sr-only">List back</span></th>
          </tr>
        </thead>
        <tbody role="rowgroup">
          <tr v-for="r in rows" :key="r.allocation_id" role="row" :class="{ revoked: r.revoked }">
            <td role="cell" class="head">
              {{ r.submitter }}
              <span v-if="r.revoked" class="badge-hud badge-archived" style="margin-left:4px;">REVOKED</span>
              <span class="muted" style="display:block;">{{ r.list_name }}</span>
            </td>
            <td role="cell" class="num" data-label="QUOTA">{{ r.quota }}</td>
            <td role="cell" class="num" data-label="ARRIVED / GOING">{{ countPair(r.arrived, r.going, 'going') }}</td>
            <td role="cell" class="num" data-label="NO-SHOW">{{ formatPercent(r.no_show_rate) }}</td>
            <td role="cell" class="num" data-label="HEADS CHECKED IN">{{ r.heads_admitted }}</td>
            <td role="cell" class="wide" style="text-align:right;">
              <button
                type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;white-space:nowrap;"
                :aria-label="r.going === 0 ? `No guests to list back for ${r.submitter}` : `List back CSV for ${r.submitter}`"
                :aria-busy="busy.has(r.allocation_id)"
                :disabled="r.going === 0 || busy.has(r.allocation_id)"
                :data-testid="`list-back-${r.allocation_id}`"
                @click="emit('listBack', r)"
              >
                <Download v-if="r.going > 0" style="width:14px;height:14px;" aria-hidden="true" /> {{ buttonLabel(r) }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.block { padding: 12px; min-width: 0; scroll-margin-top: 16px; }
/* Revoked allocations stay listed (and can still be listed back), muted. */
.revoked td { color: var(--color-on-surface-variant); }
</style>
