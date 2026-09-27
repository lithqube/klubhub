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
    <table class="report-table stack" data-testid="report-tickets">
      <thead>
        <tr>
          <th scope="col">TYPE</th>
          <th scope="col" class="num">SCANNED</th>
          <th scope="col" class="num">VALID</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="t in rows" :key="t.ticket_type_id">
          <td class="head">{{ t.name }}</td>
          <td class="num" data-label="SCANNED">{{ t.scanned }}</td>
          <td class="num" data-label="VALID">{{ t.valid }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.block { padding: 12px; min-width: 0; }
</style>
