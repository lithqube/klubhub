<script setup lang="ts">
import { computed, ref } from 'vue'
import { Plus, Trash2 } from 'lucide-vue-next'
import type { Deadline } from '~/types/release'

defineProps<{
  deadlines: Deadline[]
  busy?: boolean
}>()

const emit = defineEmits<{
  add: [name: string, dueDate: string]
  complete: [id: string]
  remove: [id: string]
}>()

const name = ref('')
const dueDate = ref('')

const open = computed(() => name.value.trim().length > 0 && dueDate.value.length > 0)

function submit(): void {
  if (!open.value) return
  emit('add', name.value.trim(), dueDate.value)
  name.value = ''
  dueDate.value = ''
}
</script>

<template>
  <section class="border border-outline/50 p-3" data-testid="release-deadlines">
    <h3 class="font-terminal text-[10px] uppercase tracking-terminal text-tertiary mb-2">
      deadlines
    </h3>

    <p v-if="!deadlines.length" class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary mb-2">
      No deadlines yet.
    </p>

    <ul v-else class="mb-3 space-y-1">
      <li
        v-for="deadline in deadlines"
        :key="deadline.id"
        class="flex items-center justify-between gap-2"
      >
        <button
          type="button"
          class="flex items-center gap-2 font-terminal text-[10px] uppercase tracking-terminal"
          :class="deadline.completed ? 'text-tertiary line-through' : 'text-foreground'"
          :disabled="busy"
          @click="emit('complete', deadline.id)"
        >
          <span
            class="inline-block w-3 h-3 border"
            :class="deadline.completed ? 'bg-primary border-primary' : 'border-outline/70'"
            aria-hidden="true"
          />
          {{ deadline.name }}
        </button>
        <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">
          {{ deadline.due_date }}
        </span>
        <button
          type="button"
          class="text-tertiary hover:text-error transition-colors"
          :disabled="busy"
          :aria-label="`Remove ${deadline.name}`"
          @click="emit('remove', deadline.id)"
        >
          <Trash2 class="w-3 h-3" />
        </button>
      </li>
    </ul>

    <div class="flex gap-2 items-end">
      <label class="flex-1">
        <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">Step</span>
        <input
          v-model="name"
          type="text"
          class="w-full mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
          data-testid="deadline-name"
        >
      </label>
      <label>
        <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">Due</span>
        <input
          v-model="dueDate"
          type="date"
          class="mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
          data-testid="deadline-due"
        >
      </label>
      <button
        type="button"
        class="flex items-center gap-1 px-2 py-1 border border-primary text-primary font-terminal text-[9px] uppercase tracking-terminal disabled:opacity-50"
        :disabled="!open || busy"
        data-testid="deadline-add"
        @click="submit"
      >
        <Plus class="w-3 h-3" /> Add
      </button>
    </div>
  </section>
</template>
