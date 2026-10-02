<script setup lang="ts">
// RiderSectionField — reusable section: label + quick-insert chips +
// textarea. Mirrors EpkTechRiderSection.vue's chip pattern. Used by
// both RiderTemplateEditor and RiderAttachmentEditor.

import { computed } from 'vue'
import Textarea from '#kui/components/ui/textarea/Textarea.vue'
import { SECTION_CHIPS } from './chips'
import type { RiderSection } from '~/types/rider'

const props = defineProps<{
  section: RiderSection
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const textareaProxy = computed({
  get: () => props.modelValue,
  set: (v: string) => emit('update:modelValue', v),
})

const chips = computed(() => SECTION_CHIPS[props.section] ?? [])

function insertChip(text: string): void {
  const trimmed = textareaProxy.value.trim()
  textareaProxy.value = trimmed ? `${trimmed}, ${text}` : text
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">
      <slot name="label">{{ section.toUpperCase() }}</slot>
    </p>

    <div v-if="chips.length" class="flex flex-wrap gap-2" role="group" :aria-label="`Quick-insert ${section}`">
      <button
        v-for="chip in chips"
        :key="chip"
        type="button"
        class="social-chip"
        :data-testid="`${section}-chip-${chip}`"
        @click="insertChip(chip)"
      >
        + {{ chip }}
      </button>
    </div>

    <Textarea
      v-model="textareaProxy"
      :rows="6"
      :placeholder="`${section} requirements…`"
      :data-testid="`rider-${section}-textarea`"
    />
  </div>
</template>