<script setup lang="ts">
// RiderTemplateCreateForm — new-template composer, INLINE in the templates pane
// (no modal), in the place the template editor takes once one exists. Submit →
// store.createTemplate, then the page selects the new template and the editor
// takes over (from then on edits autosave).
//
// The host (pages/rider.vue) mounts it while a new template is being composed
// and unmounts it on `close`, so every opening starts empty.

import { computed, nextTick, onMounted, ref, useId, watch } from 'vue'
import { focusWhenReady } from '~/utils/dialogFocus'
import { useRiderStore } from '~/stores/rider'
import { RIDER_NAME_MAX_CHARS, type RiderTemplate } from '~/types/rider'
import { riderErrorMessage } from '~/utils/riderErrors'
import RiderSectionField from './RiderSectionField.vue'

const props = defineProps<{
  /** Bumped by the host each time the composer is (re)requested: pull focus back to the name. */
  focusSeq?: number
}>()
const emit = defineEmits<{ close: []; created: [template: RiderTemplate] }>()

const titleId = useId()
const nameId = useId()
const root = ref<HTMLElement | null>(null)
const nameEl = ref<HTMLInputElement | null>(null)

const store = useRiderStore()

const name = ref('')
const technical = ref('')
const hospitality = ref('')
const backline = ref('')
const otherNotes = ref('')
const error = ref('')
const submitting = ref(false)

/** Nothing typed yet: closing loses nothing. */
const pristine = computed(() =>
  ![name, technical, hospitality, backline, otherNotes].some((f) => f.value.trim() !== ''),
)

function focusName(): void {
  focusWhenReady(nameEl)
}

onMounted(() => {
  focusName()
  // On a phone the pane sits below the template list: bring it into view.
  void nextTick(() => root.value?.scrollIntoView?.({ block: 'nearest' }))
})
watch(() => props.focusSeq, focusName)

async function onSubmit(): Promise<void> {
  if (submitting.value) return
  error.value = ''
  if (!name.value.trim()) {
    error.value = 'Name is required.'
    nameEl.value?.focus()
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

function onEscape(e: KeyboardEvent): void {
  // Free text is long: an accidental Escape must not throw a draft away.
  // CANCEL is the deliberate way out once something is typed.
  if (e.defaultPrevented || !pristine.value) return
  e.stopPropagation()
  emit('close')
}
</script>

<template>
  <section
    ref="root"
    class="space-y-4"
    data-testid="rider-template-create"
    :aria-labelledby="titleId"
    @keydown.esc="onEscape"
  >
    <header>
      <h2 :id="titleId" class="font-command font-bold uppercase text-on-surface text-lg tracking-command">NEW TEMPLATE</h2>
      <p class="rtc-hint">Saved when you press CREATE TEMPLATE. After that, changes save automatically.</p>
    </header>

    <div class="glass-panel p-4 space-y-2">
      <label :for="nameId" class="text-xs tracking-terminal text-tertiary uppercase font-terminal">TEMPLATE NAME</label>
      <input
        :id="nameId"
        ref="nameEl"
        v-model="name"
        class="hud-input"
        style="width:100%;"
        placeholder="Standard club"
        :maxlength="RIDER_NAME_MAX_CHARS"
        :aria-invalid="error && !name.trim() ? 'true' : undefined"
        data-testid="rider-template-create-name"
        @keydown.enter.prevent="onSubmit"
      >
    </div>

    <RiderSectionField v-model="technical" section="technical" />
    <RiderSectionField v-model="hospitality" section="hospitality" />
    <RiderSectionField v-model="backline" section="backline" />
    <RiderSectionField v-model="otherNotes" section="otherNotes" />

    <p v-if="error" role="alert" class="text-error font-terminal text-xs uppercase" data-testid="rider-template-create-error">{{ error }}</p>

    <div class="rtc-actions">
      <button type="button" class="btn-hud btn-hud-ghost" data-testid="rider-template-create-cancel" @click="emit('close')">CANCEL</button>
      <button
        type="button"
        class="btn-hud btn-hud-cta"
        :disabled="submitting"
        data-testid="rider-template-create-submit"
        @click="onSubmit"
      >
        {{ submitting ? 'CREATING…' : 'CREATE TEMPLATE' }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.rtc-hint { margin: 4px 0 0; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface-variant); }
.rtc-actions { display: flex; justify-content: flex-end; gap: 8px; padding-top: 14px; border-top: 1px dashed color-mix(in srgb, var(--color-on-surface) 14%, transparent); }
.btn-hud:disabled { opacity: .45; cursor: not-allowed; box-shadow: none; transform: none; }
@media (max-width: 768px) {
  .rtc-actions { flex-direction: column-reverse; }
  .rtc-actions .btn-hud { width: 100%; min-height: 44px; }
}
</style>
