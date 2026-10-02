<script setup lang="ts">
// RiderTemplateList — list of `.glass` rows for templates. Click a row
// → emits `select` so the parent swaps the editor pane.

defineProps<{ templates: { id: string; name: string }[]; selectedId?: string | null }>()
const emit = defineEmits<{ select: [id: string] }>()
</script>

<template>
  <div class="space-y-2" data-testid="rider-template-list">
    <div v-if="templates.length === 0" class="text-tertiary font-terminal uppercase text-xs">
      No templates yet. Create one with + NEW TEMPLATE.
    </div>
    <button
      v-for="t in templates"
      :key="t.id"
      type="button"
      class="glass-panel w-full text-left p-3 hover:opacity-90"
      :aria-current="t.id === selectedId ? 'true' : undefined"
      :class="{ 'is-selected': t.id === selectedId }"
      :data-testid="`rider-template-row-${t.id}`"
      @click="emit('select', t.id)"
    >
      <div class="font-terminal text-on-surface uppercase text-sm tracking-terminal">
        {{ t.name }}
      </div>
    </button>
  </div>
</template>

<style scoped>
.is-selected {
  outline: 1px solid var(--color-primary);
  outline-offset: -1px;
}
</style>
