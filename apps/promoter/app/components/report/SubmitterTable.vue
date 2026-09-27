<script setup lang="ts">
import { Download } from 'lucide-vue-next'
import type { ReportSubmitterRow } from '~/types/report'
import { formatPercent, sortSubmitters } from '~/utils/report'

/**
 * Arrivals by allocation (P2.4), artist lists first. LIST BACK CSV sends
 * a submitter who came of their guests: names, +N, arrival and first-in
 * time; never email or phone.
 */
const props = defineProps<{ rows: ReportSubmitterRow[], busy: string | null }>()
const emit = defineEmits<{ listBack: [row: ReportSubmitterRow] }>()
const rows = computed(() => sortSubmitters(props.rows))
</script>

<template>
  <section class="glass block" aria-labelledby="report-subs-h">
    <h2 id="report-subs-h" class="section-lbl" style="margin:0 0 6px;">BY SUBMITTER</h2>
    <p v-if="!rows.length" style="margin:0;font-size:13px;color:var(--color-on-surface-variant);">No allocations on this event.</p>
    <table v-else class="report-table stack" data-testid="report-submitters">
      <thead>
        <tr>
          <th scope="col">SUBMITTER</th>
          <th scope="col" class="num">QUOTA</th>
          <th scope="col" class="num">ARRIVED / GOING</th>
          <th scope="col" class="num">NO-SHOW</th>
          <th scope="col" class="num">HEADS IN</th>
          <th scope="col"><span class="sr-only">List back</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.allocation_id" :class="{ revoked: r.revoked }">
          <td class="head">
            {{ r.submitter }}
            <span v-if="r.revoked" class="badge-hud badge-published" style="margin-left:4px;">REVOKED</span>
            <span class="muted" style="display:block;">{{ r.list_name }}</span>
          </td>
          <td class="num" data-label="QUOTA">{{ r.quota }}</td>
          <td class="num" data-label="ARRIVED / GOING">{{ r.arrived }} / {{ r.going }}</td>
          <td class="num" data-label="NO-SHOW">{{ formatPercent(r.no_show_rate) }}</td>
          <td class="num" data-label="HEADS IN">{{ r.heads_admitted }}</td>
          <td class="wide" style="text-align:right;">
            <button
              type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;white-space:nowrap;"
              :aria-label="`List back CSV for ${r.submitter}`" :disabled="busy === r.allocation_id" @click="emit('listBack', r)"
            >
              <Download style="width:14px;height:14px;" aria-hidden="true" /> {{ busy === r.allocation_id ? 'PREPARING…' : 'LIST BACK CSV' }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.block { padding: 12px; min-width: 0; }
/* Revoked allocations stay listed (and can still be listed back), muted. */
.revoked td { color: var(--color-on-surface-variant); }
</style>
