<script setup lang="ts">
import type { ReportTicketType } from '~/types/report'

/**
 * Imported tickets by type (P2.4). SCANNED counts tickets of any status with
 * a live check-in, VALID only valid ones, so scanned can exceed valid: counts
 * only, never a percentage.
 */
defineProps<{ rows: ReportTicketType[] }>()
</script>

<template>
  <section class="glass block" aria-labelledby="report-tickets-h">
    <h2 id="report-tickets-h" class="section-lbl" style="margin:0 0 6px;">TICKETS BY TYPE</h2>
    <div class="report-table-wrap">
      <table role="table" class="report-table stack" data-testid="report-tickets">
        <thead role="rowgroup">
          <tr role="row">
            <th role="columnheader" scope="col">TYPE</th>
            <th role="columnheader" scope="col" class="num">SCANNED</th>
            <th role="columnheader" scope="col" class="num">VALID</th>
          </tr>
        </thead>
        <tbody role="rowgroup">
          <tr v-for="t in rows" :key="t.ticket_type_id" role="row">
            <td role="cell" class="head">{{ t.name }}</td>
            <td role="cell" class="num" data-label="SCANNED">{{ t.scanned }}</td>
            <td role="cell" class="num" data-label="VALID">{{ t.valid }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.block { padding: 12px; min-width: 0; }
</style>
