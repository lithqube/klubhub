<script setup lang="ts">
// RiderTemplateEditor — name field + four RiderSectionField stack.
// Calls useRiderAutosave with the template id so changes debounce.

import { onBeforeUnmount, ref, watch } from 'vue'
import { useRiderAutosave } from '~/composables/useRiderAutosave'
import { useRiderStore } from '~/stores/rider'
import type { RiderTemplate } from '~/types/rider'
import RiderConflictNotice from './RiderConflictNotice.vue'
import RiderSectionField from './RiderSectionField.vue'

const props = defineProps<{ template: RiderTemplate }>()

const name = ref(props.template.name)
const technical = ref(props.template.technical)
const hospitality = ref(props.template.hospitality)
const backline = ref(props.template.backline)
const otherNotes = ref(props.template.otherNotes)

const store = useRiderStore()
const { scheduleSave, flush } = useRiderAutosave()

// Reset local state to the template's values when the parent swaps to a
// different template (user clicks a different row in the list), and when the
// user explicitly discards their edits to take the server's copy after a
// conflict. Not on every store update: our own saves refresh the cached
// template, and reseeding then would clobber text typed during the request.
// Separate sources, not one getter returning an array: a fresh array always
// counts as "changed", so it would fire whenever the parent passes the
// refreshed template object, i.e. after every save.
watch([() => props.template.id, () => store.reloadVersion], () => {
  name.value = props.template.name
  technical.value = props.template.technical
  hospitality.value = props.template.hospitality
  backline.value = props.template.backline
  otherNotes.value = props.template.otherNotes
})

function onName(value: string): void {
  name.value = value
  scheduleSave({ kind: 'template', id: props.template.id }, { name: value })
}

function onSection(field: 'technical' | 'hospitality' | 'backline' | 'otherNotes', value: string): void {
  // Use v-model on the textarea; this is the @update:modelValue handler
  // for the local section ref. Save the whole four-section snapshot so
  // COALESCE-style partial updates keep untouched fields stable (matches
  // API contract documented in api/internal/rider/handler.go).
  const snapshot = {
    technical: technical.value,
    hospitality: hospitality.value,
    backline: backline.value,
    otherNotes: otherNotes.value,
    [field]: value,
  }
  scheduleSave(
    { kind: 'template', id: props.template.id },
    {
      technical: snapshot.technical,
      hospitality: snapshot.hospitality,
      backline: snapshot.backline,
      otherNotes: snapshot.otherNotes,
    },
  )
}

onBeforeUnmount(() => {
  void flush({ kind: 'template', id: props.template.id })
})
</script>

<template>
  <div class="space-y-4">
    <RiderConflictNotice :id="template.id" kind="template" />
    <div class="glass-panel p-4 space-y-2">
      <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">TEMPLATE NAME</p>
      <input
        :value="name"
        class="hud-input"
        style="width:100%;"
        placeholder="Standard club"
        data-testid="rider-template-name-input"
        @input="onName(($event.target as HTMLInputElement).value)"
      >
    </div>

    <RiderSectionField
      section="technical"
      :model-value="technical"
      @update:model-value="(v: string) => { technical = v; onSection('technical', v) }"
    />
    <RiderSectionField
      section="hospitality"
      :model-value="hospitality"
      @update:model-value="(v: string) => { hospitality = v; onSection('hospitality', v) }"
    />
    <RiderSectionField
      section="backline"
      :model-value="backline"
      @update:model-value="(v: string) => { backline = v; onSection('backline', v) }"
    />
    <RiderSectionField
      section="otherNotes"
      :model-value="otherNotes"
      @update:model-value="(v: string) => { otherNotes = v; onSection('otherNotes', v) }"
    />
  </div>
</template>