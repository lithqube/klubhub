<script setup lang="ts">
import type { CopyField } from '~/utils/exportPack'

const props = defineProps<{ field: CopyField }>()
const lines = computed(() => props.field.value.split('\n').length)
const meta = computed(() => {
  if (props.field.withheld) return 'WITHHELD'
  if (!props.field.value) return 'EMPTY'
  return lines.value > 1 ? `${lines.value} LINES` : `${props.field.value.length} CHARS`
})
</script>

<template>
  <div style="display:grid;grid-template-columns:minmax(0,1fr) auto;gap:6px 10px;align-items:start;padding:8px 0;border-bottom:1px dashed var(--color-outline-variant);">
    <div style="min-width:0;">
      <div style="display:flex;gap:8px;align-items:baseline;">
        <span class="section-lbl" style="margin:0;">{{ field.label }}</span>
        <span class="data-frag" style="font-size:8px;" :style="field.withheld ? 'color:var(--color-secondary)' : ''">{{ meta }}</span>
      </div>
      <div style="font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere;max-height:8.5em;overflow:auto;margin-top:2px;user-select:text;">{{ field.value || '—' }}</div>
    </div>
    <ExportCopyButton v-if="field.value" :text="field.value" :label="`Copy ${field.label.toLowerCase()}`" />
  </div>
</template>
