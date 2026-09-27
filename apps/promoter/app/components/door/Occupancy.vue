<script setup lang="ts">
import { LogOut, UserPlus } from 'lucide-vue-next'
import type { Occupancy } from '~/utils/doorState'

/** Heads inside vs. capacity, with the walk-up (+1 in, paid at the door) and OUT (+1 left) counters. */
defineProps<{ occ: Occupancy }>()
const emit = defineEmits<{ walkup: [], out: [] }>()
</script>

<template>
  <section class="occ glass" aria-label="Occupancy">
    <div class="nums">
      <span class="section-lbl">INSIDE</span>
      <span class="count" data-testid="door-inside">{{ occ.inside }}</span>
      <span v-if="occ.capacity" class="cap">/ {{ occ.capacity }}</span>
      <span v-if="occ.tone === 'full'" class="data-frag" style="color:var(--color-error);">FULL</span>
      <span v-else-if="occ.tone === 'near'" class="data-frag" style="color:var(--color-status-archived);">NEAR CAPACITY</span>
    </div>
    <div
      v-if="occ.pct !== null" class="prog-track" role="progressbar" :aria-valuenow="occ.inside" aria-valuemin="0"
      :aria-valuemax="occ.capacity ?? undefined" aria-label="Occupancy" style="height:6px;"
    >
      <div class="prog-fill" :class="`tone-${occ.tone}`" :style="{ width: `${occ.pct}%` }" />
    </div>
    <div class="btns">
      <button type="button" class="btn-hud btn-hud-ghost big" @click="emit('walkup')">
        <UserPlus style="width:18px;height:18px;" aria-hidden="true" /> WALK-UP +1
      </button>
      <button type="button" class="btn-hud btn-hud-ghost big" @click="emit('out')">
        <LogOut style="width:18px;height:18px;" aria-hidden="true" /> OUT −1
      </button>
    </div>
  </section>
</template>

<style scoped>
.occ {
  padding: 10px 12px;
  display: grid;
  gap: 8px;
}
.nums {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 8px;
}
.count {
  font-family: var(--font-command);
  font-size: 28px;
  font-weight: 700;
  color: var(--color-on-surface);
}
.cap {
  font-family: var(--font-command);
  font-size: 16px;
  color: var(--color-on-surface-variant);
}
.tone-near { background: var(--color-status-archived); }
.tone-full { background: var(--color-error); }
.btns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.big {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
}
</style>
