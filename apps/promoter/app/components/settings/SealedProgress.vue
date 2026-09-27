<script setup lang="ts">
import { progressText } from '~/utils/sealedText'

/**
 * The "working" line while the passphrase key is derived (Argon2id takes a
 * second on a laptop and several on a phone): a bar with its percentage,
 * announced politely once, not on every step.
 */
const props = defineProps<{ label: string, progress: number | null }>()
const pct = computed(() => (props.progress === null ? null : Math.round(Math.min(1, Math.max(0, props.progress)) * 100)))
</script>

<template>
  <div class="wrap" data-testid="sealed-working">
    <p class="lbl" role="status">{{ label }}</p>
    <div
      class="bar" role="progressbar" :aria-label="label" aria-valuemin="0" aria-valuemax="100"
      :aria-valuenow="pct ?? undefined" :aria-valuetext="pct === null ? 'working' : progressText(progress)"
    >
      <span class="fill" :class="{ busy: pct === null }" :style="pct === null ? undefined : { width: `${pct}%` }" />
    </div>
    <span class="pct" aria-hidden="true">{{ progressText(progress) }}</span>
  </div>
</template>

<style scoped>
.wrap {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 4px 10px;
  align-items: center;
}
.lbl {
  grid-column: 1 / -1;
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-tertiary);
}
.bar {
  height: 6px;
  background: var(--color-surface-container-high);
  overflow: hidden;
}
.fill {
  display: block;
  height: 100%;
  background: var(--color-primary);
  transition: width .2s linear;
}
.fill.busy {
  width: 30%;
  animation: sealed-busy 1.2s ease-in-out infinite;
}
@keyframes sealed-busy {
  from { transform: translateX(-100%); }
  to { transform: translateX(340%); }
}
@media (prefers-reduced-motion: reduce) {
  .fill { transition: none; }
  .fill.busy { animation: none; width: 100%; opacity: .4; }
}
.pct {
  font-family: var(--font-terminal);
  font-size: 11px;
  color: var(--color-on-surface-variant);
  min-width: 36px;
  text-align: right;
}
</style>
