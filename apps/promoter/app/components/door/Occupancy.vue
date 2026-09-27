<script setup lang="ts">
import type { Occupancy } from '~/utils/doorState'

/** Heads inside vs. capacity, compact for the sticky door header: "INSIDE 212/300" and a thin bar. */
defineProps<{ occ: Occupancy }>()
</script>

<template>
  <div class="occ" role="group" aria-label="Occupancy">
    <p class="nums">
      <span class="lbl">INSIDE</span>
      <span class="count" data-testid="door-inside">{{ occ.inside }}</span><span v-if="occ.capacity" class="cap">/{{ occ.capacity }}</span>
      <span v-if="occ.tone === 'full'" class="tone" style="color:var(--color-error);">FULL</span>
      <span v-else-if="occ.tone === 'near'" class="tone" style="color:var(--color-status-archived);">NEAR CAP</span>
    </p>
    <div
      v-if="occ.pct !== null" class="prog-track" role="progressbar" :aria-valuenow="occ.inside" aria-valuemin="0"
      :aria-valuemax="occ.capacity ?? undefined" aria-label="Occupancy" style="height:3px;"
    >
      <div class="prog-fill" :class="`tone-${occ.tone}`" :style="{ width: `${occ.pct}%` }" />
    </div>
  </div>
</template>

<style scoped>
.occ {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.nums {
  margin: 0;
  display: flex;
  align-items: baseline;
  gap: 4px;
  white-space: nowrap;
  font-family: var(--font-terminal);
}
.lbl {
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-on-surface-variant);
}
.count {
  font-family: var(--font-command);
  font-size: 16px;
  font-weight: 700;
  color: var(--color-on-surface);
}
.cap {
  font-family: var(--font-command);
  font-size: 13px;
  color: var(--color-on-surface-variant);
}
.tone {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: .05em;
}
.tone-near { background: var(--color-status-archived); }
.tone-full { background: var(--color-error); }
</style>
