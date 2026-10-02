<script setup lang="ts">
// RiderTemplateDialog — create-new modal. Submit → store.createTemplate.

import { ref, useId, watch } from 'vue'
import { useModalA11y } from '~/composables/useModalA11y'
import { useRiderStore } from '~/stores/rider'
import { RIDER_NAME_MAX_CHARS, type RiderTemplate } from '~/types/rider'
import { riderErrorMessage } from '~/utils/riderErrors'
import RiderSectionField from './RiderSectionField.vue'

// `open` must be a declared prop: the template reads it, and an undeclared
// attribute is not visible to the template, so the dialog never rendered.
const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; created: [template: RiderTemplate] }>()

const titleId = useId()
const nameId = useId()
const panel = ref<HTMLElement | null>(null)
useModalA11y(() => props.open, panel, () => emit('close'))

const store = useRiderStore()

const name = ref('')
const technical = ref('')
const hospitality = ref('')
const backline = ref('')
const otherNotes = ref('')
const error = ref('')
const submitting = ref(false)

// The component stays mounted while closed: clear it so a reopened dialog
// does not show the previous values (and invite a duplicate create).
watch(() => props.open, (isOpen) => {
  if (isOpen) return
  name.value = ''
  technical.value = ''
  hospitality.value = ''
  backline.value = ''
  otherNotes.value = ''
  error.value = ''
  submitting.value = false
})

async function onSubmit(): Promise<void> {
  error.value = ''
  if (!name.value.trim()) {
    error.value = 'Name is required.'
    return
  }
  submitting.value = true
  try {
    const created = await store.createTemplate({
      name: name.value.trim(),
      technical: technical.value,
      hospitality: hospitality.value,
      backline: backline.value,
      otherNotes: otherNotes.value,
    })
    emit('created', created)
    emit('close')
  } catch (e: unknown) {
    error.value = riderErrorMessage(e, 'Could not create the template.')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div
    v-if="open"
    data-testid="rider-template-dialog"
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60"
    role="dialog"
    aria-modal="true"
    :aria-labelledby="titleId"
    @click.self="emit('close')"
  >
    <div ref="panel" tabindex="-1" class="glass-panel w-full max-w-2xl max-h-[90vh] overflow-y-auto p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 :id="titleId" class="font-command font-bold uppercase text-on-surface text-lg tracking-command">
          NEW TEMPLATE
        </h2>
        <button type="button" class="btn-hud btn-hud-ghost" aria-label="Close" @click="emit('close')">✕</button>
      </div>

      <div>
        <label :for="nameId" class="section-lbl" style="display:block;margin-bottom:6px;">NAME</label>
        <input
          :id="nameId"
          v-model="name"
          data-autofocus
          class="hud-input"
          style="width:100%;"
          placeholder="Standard club"
          :maxlength="RIDER_NAME_MAX_CHARS"
          data-testid="rider-template-dialog-name"
        >
      </div>

      <RiderSectionField section="technical" v-model="technical" />
      <RiderSectionField section="hospitality" v-model="hospitality" />
      <RiderSectionField section="backline" v-model="backline" />
      <RiderSectionField section="otherNotes" v-model="otherNotes" />

      <p v-if="error" role="alert" class="text-error font-terminal text-xs uppercase">{{ error }}</p>

      <div class="flex justify-end gap-2">
        <button type="button" class="btn-hud btn-hud-ghost" @click="emit('close')">CANCEL</button>
        <button
          type="button"
          class="btn-hud btn-hud-cta"
          :disabled="submitting"
          data-testid="rider-template-dialog-submit"
          @click="onSubmit"
        >
          {{ submitting ? 'CREATING…' : 'CREATE TEMPLATE' }}
        </button>
      </div>
    </div>
  </div>
</template>