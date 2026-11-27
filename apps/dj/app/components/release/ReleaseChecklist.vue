<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Trash2, Save } from 'lucide-vue-next'
import type { ChecklistItem } from '~/types/release'

defineProps<{
  items: ChecklistItem[]
  busy?: boolean
}>()

const emit = defineEmits<{
  add: [label: string]
  toggle: [id: string, done: boolean]
  remove: [id: string]
  saveAsDefault: []
}>()

const label = ref('')

function add(): void {
  if (!label.value.trim()) return
  emit('add', label.value.trim())
  label.value = ''
}
</script>

<template>
  <section class="border border-outline/50 p-3" data-testid="release-checklist">
    <div class="flex items-center justify-between mb-2">
      <h3 class="font-terminal text-[10px] uppercase tracking-terminal text-tertiary">
        promo checklist
      </h3>
      <button
        type="button"
        class="flex items-center gap-1 px-2 py-1 border border-outline/60 font-terminal text-[9px] uppercase tracking-terminal text-tertiary hover:text-primary transition-colors"
        :disabled="busy"
        data-testid="checklist-save-default"
        @click="emit('saveAsDefault')"
      >
        <Save class="w-3 h-3" /> Save as default
      </button>
    </div>

    <ul v-if="items.length" class="mb-3 space-y-1">
      <li
        v-for="item in items"
        :key="item.id"
        class="flex items-center justify-between gap-2"
      >
        <button
          type="button"
          class="flex items-center gap-2 font-terminal text-[10px] uppercase tracking-terminal"
          :class="item.done ? 'text-tertiary line-through' : 'text-foreground'"
          :disabled="busy"
          @click="emit('toggle', item.id, !item.done)"
        >
          <span
            class="inline-block w-3 h-3 border"
            :class="item.done ? 'bg-primary border-primary' : 'border-outline/70'"
            aria-hidden="true"
          />
          {{ item.label }}
        </button>
        <button
          type="button"
          class="text-tertiary hover:text-error transition-colors"
          :disabled="busy"
          :aria-label="`Remove ${item.label}`"
          @click="emit('remove', item.id)"
        >
          <Trash2 class="w-3 h-3" />
        </button>
      </li>
    </ul>
    <p v-else class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary mb-2">
      No steps yet.
    </p>

    <div class="flex gap-2 items-end">
      <label class="flex-1">
        <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">New step</span>
        <input
          v-model="label"
          type="text"
          class="w-full mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
          data-testid="checklist-label"
        >
      </label>
      <button
        type="button"
        class="flex items-center gap-1 px-2 py-1 border border-primary text-primary font-terminal text-[9px] uppercase tracking-terminal disabled:opacity-50"
        :disabled="!label.trim() || busy"
        data-testid="checklist-add"
        @click="add"
      >
        <Plus class="w-3 h-3" /> Add
      </button>
    </div>
  </section>
</template>
