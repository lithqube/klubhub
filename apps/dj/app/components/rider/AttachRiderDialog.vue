<script setup lang="ts">
// AttachRiderDialog — pick a template (or "Start from blank") for a
// gig. Used inside GigFormDialog when status flips to advanced.

import { computed, ref, watch } from 'vue'
import { useRiderStore } from '~/stores/rider'
import { riderErrorMessage } from '~/utils/riderErrors'

const props = defineProps<{ open: boolean; gigId: string }>()
const emit = defineEmits<{ close: []; attached: [] }>()

const store = useRiderStore()
const selectedTemplateId = ref<string | null>(null)
const error = ref('')
const submitting = ref(false)

// store.templates is otherwise only filled by /rider; opened from the gig
// form first, the picker would offer only "Start from blank".
watch(() => props.open, async (isOpen) => {
  if (!isOpen || store.templates.length > 0) return
  try {
    await store.loadTemplates()
  } catch {
    error.value = 'Could not load rider templates. You can still start from blank.'
  }
}, { immediate: true })

const previewTemplate = computed(() =>
  selectedTemplateId.value ? store.templateById.get(selectedTemplateId.value) ?? null : null,
)

async function onConfirm(): Promise<void> {
  error.value = ''
  submitting.value = true
  try {
    await store.createAttachment({
      gigId: props.gigId,
      templateId: selectedTemplateId.value,
    })
    emit('attached')
  } catch (e: unknown) {
    const err = e as { statusCode?: number; status?: number }
    if ((err.statusCode ?? err.status) === 409) {
      // One rider per gig: it already exists, so refresh instead of failing.
      emit('attached')
      return
    }
    error.value = riderErrorMessage(e, 'Could not attach the rider.')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div
    v-if="open"
    data-testid="attach-rider-dialog"
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60"
    role="dialog"
    aria-modal="true"
    @click.self="emit('close')"
  >
    <div class="glass-panel w-full max-w-3xl max-h-[90vh] overflow-y-auto p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="font-command font-bold uppercase text-on-surface text-lg tracking-command">
          ATTACH RIDER
        </h2>
        <button type="button" class="btn-hud btn-hud-ghost" @click="emit('close')">✕</button>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2 max-h-80 overflow-y-auto" data-testid="attach-rider-template-list">
          <button
            type="button"
            class="glass-panel w-full text-left p-3"
            :class="{ 'is-selected': selectedTemplateId === null }"
            :data-testid="'attach-rider-blank'"
            @click="selectedTemplateId = null"
          >
            <div class="font-terminal text-on-surface uppercase text-sm tracking-terminal">
              START FROM BLANK
            </div>
            <p class="font-terminal text-tertiary text-xs uppercase mt-1">Empty sections, fill in manually.</p>
          </button>

          <button
            v-for="t in store.templates"
            :key="t.id"
            type="button"
            class="glass-panel w-full text-left p-3"
            :class="{ 'is-selected': selectedTemplateId === t.id }"
            :data-testid="`attach-rider-template-${t.id}`"
            @click="selectedTemplateId = t.id"
          >
            <div class="font-terminal text-on-surface uppercase text-sm tracking-terminal">
              {{ t.name }}
            </div>
            <p v-if="t.technical" class="font-terminal text-tertiary text-xs uppercase mt-1 truncate">
              {{ t.technical.split('\n')[0] }}
            </p>
          </button>
        </div>

        <div class="space-y-2">
          <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">PREVIEW</p>
          <div v-if="!previewTemplate" class="glass-panel p-4 text-tertiary font-terminal uppercase text-xs">
            Empty sections. Type the four fields manually after attaching.
          </div>
          <div v-else class="glass-panel p-4 space-y-2">
            <div v-if="previewTemplate.technical">
              <p class="text-xs text-tertiary font-terminal uppercase">TECHNICAL</p>
              <pre class="font-terminal text-xs whitespace-pre-wrap">{{ previewTemplate.technical }}</pre>
            </div>
            <div v-if="previewTemplate.hospitality">
              <p class="text-xs text-tertiary font-terminal uppercase">HOSPITALITY</p>
              <pre class="font-terminal text-xs whitespace-pre-wrap">{{ previewTemplate.hospitality }}</pre>
            </div>
            <div v-if="previewTemplate.backline">
              <p class="text-xs text-tertiary font-terminal uppercase">BACKLINE</p>
              <pre class="font-terminal text-xs whitespace-pre-wrap">{{ previewTemplate.backline }}</pre>
            </div>
            <div v-if="previewTemplate.otherNotes">
              <p class="text-xs text-tertiary font-terminal uppercase">OTHER NOTES</p>
              <pre class="font-terminal text-xs whitespace-pre-wrap">{{ previewTemplate.otherNotes }}</pre>
            </div>
          </div>
        </div>
      </div>

      <p v-if="error" class="text-error font-terminal text-xs uppercase">{{ error }}</p>

      <div class="flex justify-end gap-2">
        <button type="button" class="btn-hud btn-hud-ghost" @click="emit('close')">CANCEL</button>
        <button
          type="button"
          class="btn-hud btn-hud-cta"
          :disabled="submitting"
          data-testid="attach-rider-confirm"
          @click="onConfirm"
        >
          {{ submitting ? 'ATTACHING…' : 'ATTACH' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.is-selected {
  outline: 1px solid var(--color-primary);
  outline-offset: -1px;
}
</style>