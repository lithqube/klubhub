<script setup lang="ts">
// RiderSectionField — reusable section: label + quick-insert chips +
// textarea. Mirrors EpkTechRiderSection.vue's chip pattern. Used by
// both RiderTemplateEditor and RiderAttachmentEditor.

import { computed, useId } from 'vue'
import Textarea from '#kui/components/ui/textarea/Textarea.vue'
import { SECTION_CHIPS } from './chips'
import { RIDER_SECTION_MAX_CHARS, type RiderSection } from '~/types/rider'

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
const fieldId = useId()

// Rider text is one requirement per line (the templates are written that
// way), so a chip goes on its own line. Only trailing blanks are dropped:
// leading whitespace and the user's own line breaks are theirs.
function insertChip(text: string): void {
  const current = textareaProxy.value.replace(/\s+$/, '')
  textareaProxy.value = current ? `${current}\n${text}` : text
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <label :for="fieldId" class="block text-xs tracking-terminal text-tertiary uppercase font-terminal">
      <slot name="label">{{ section.toUpperCase() }}</slot>
    </label>

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
      :id="fieldId"
      v-model="textareaProxy"
      :rows="6"
      :maxlength="RIDER_SECTION_MAX_CHARS"
      :placeholder="`${section} requirements…`"
      :data-testid="`rider-${section}-textarea`"
    />
  </div>
</template>