<script setup lang="ts">
import { quotaFill } from '~/utils/guests'

/** Allocation fill: heads used of quota, as a labelled meter. */
const props = defineProps<{ used: number, quota: number, label: string }>()
const fill = computed(() => quotaFill(props.used, props.quota))
const color = computed(() => ({ ok: 'var(--color-primary)', near: 'var(--color-status-archived)', full: 'var(--color-error)' }[fill.value.tone]))
</script>

<template>
  <div style="display:flex;align-items:center;gap:8px;min-width:0;">
    <div
      class="prog-track" role="meter" :aria-label="`${label}: ${used} of ${quota} heads`"
      :aria-valuenow="used" aria-valuemin="0" :aria-valuemax="quota" style="flex:1;height:4px;"
    >
      <div style="height:100%;transition:width .4s;" :style="{ width: `${fill.pct}%`, background: color }" />
    </div>
    <span class="data-frag" style="font-size:9px;white-space:nowrap;" :style="{ color: fill.tone === 'ok' ? undefined : color }">
      {{ used }}/{{ quota }}{{ fill.tone === 'full' ? ' FULL' : '' }}
    </span>
  </div>
</template>
