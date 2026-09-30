<script setup lang="ts">
// RiderTemplateDialog — create-new modal. Submit → store.createTemplate.

import { ref } from 'vue'
import { useRiderStore } from '~/stores/rider'
import RiderSectionField from './RiderSectionField.vue'

const emit = defineEmits<{ close: [] }>()

const store = useRiderStore()

const name = ref('')
const technical = ref('')
const hospitality = ref('')
const backline = ref('')
const otherNotes = ref('')
const error = ref('')
const submitting = ref(false)

async function onSubmit(): Promise<void> {
  error.value = ''
  if (!name.value.trim()) {
    error.value = 'Name is required.'
    return
  }
  submitting.value = true
  try {
    await store.createTemplate({
      name: name.value.trim(),
      technical: technical.value,
      hospitality: hospitality.value,
      backline: backline.value,
      otherNotes: otherNotes.value,
    })
    emit('close')
  } catch (e: unknown) {
    error.value = (e as { message?: string })?.message ?? 'Could not create template.'
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
    @click.self="emit('close')"
  >
    <div class="glass-panel w-full max-w-2xl max-h-[90vh] overflow-y-auto p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="font-command font-bold uppercase text-on-surface text-lg tracking-command">
          NEW TEMPLATE
        </h2>
        <button type="button" class="btn-hud btn-hud-ghost" @click="emit('close')">✕</button>
      </div>

      <div>
        <label class="section-lbl" style="display:block;margin-bottom:6px;">NAME</label>
        <input
          v-model="name"
          class="hud-input"
          style="width:100%;"
          placeholder="Standard club"
          data-testid="rider-template-dialog-name"
        >
      </div>

      <RiderSectionField section="technical" v-model="technical" />
      <RiderSectionField section="hospitality" v-model="hospitality" />
      <RiderSectionField section="backline" v-model="backline" />
      <RiderSectionField section="otherNotes" v-model="otherNotes" />

      <p v-if="error" class="text-error font-terminal text-xs uppercase">{{ error }}</p>

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