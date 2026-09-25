<script setup lang="ts">
defineProps<{ title: string, code: string, filename?: string, meta?: string }>()
defineEmits<{ download: [] }>()
</script>

<template>
  <section class="hud-card" style="padding:12px 14px;min-width:0;">
    <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;justify-content:space-between;">
      <h3 class="section-lbl" style="margin:0;">{{ title }} <span v-if="meta" class="data-frag" style="font-size:8px;margin-left:6px;">{{ meta }}</span></h3>
      <div style="display:flex;gap:6px;">
        <slot name="actions" />
        <ExportCopyButton :text="code" :label="`Copy ${title.toLowerCase()}`" />
        <button v-if="filename" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="$emit('download')">{{ filename }}</button>
      </div>
    </div>
    <pre tabindex="0" :aria-label="title" style="margin:8px 0 0;max-height:220px;overflow:auto;font-size:11px;line-height:1.45;background:var(--color-surface-container-lowest, rgba(0,0,0,.25));padding:10px;white-space:pre-wrap;overflow-wrap:anywhere;"><code>{{ code }}</code></pre>
  </section>
</template>
