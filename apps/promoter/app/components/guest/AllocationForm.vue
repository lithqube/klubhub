<script setup lang="ts">
import { Lock } from 'lucide-vue-next'
import type { ApiError } from '~/types/event'
import type { Allocation, AllocationInput } from '~/types/guest'
import { instantToZoned, zonedToInstant } from '~/utils/datetime'
import { guestErrorText, MAX_PLUS_N } from '~/utils/guests'

/**
 * Create or edit a submitter's allocation (quota, +N, deadline, approval).
 * On an erased event (purged) there is no CONTACT field: the server takes
 * no personal data for it any more.
 */
const props = defineProps<{
  initial?: Allocation | null
  timezone: string
  error?: ApiError | null
  saving?: boolean
  purged?: boolean
}>()
const emit = defineEmits<{ save: [AllocationInput], cancel: [] }>()

const uid = useId()
const dl = props.initial?.deadline ? instantToZoned(props.initial.deadline, props.timezone) : null
const f = reactive({
  label: props.initial?.label ?? '',
  contact: props.initial?.submitter_contact ?? '',
  quota: props.initial?.quota ?? 10,
  plus: props.initial?.plus_n_max ?? 1,
  date: dl?.date ?? '',
  time: dl?.time ?? '18:00',
  approval: props.initial?.requires_approval ?? false,
})
const errorText = computed(() => {
  if (!props.error) return ''
  if (props.error.error === 'event_purged') return 'Guest data for this event was erased, so a submitter contact can\'t be saved. Leave it out; quota, +N and deadline still save.'
  return guestErrorText(props.error)
})

function submit() {
  emit('save', {
    label: f.label,
    submitter_contact: props.purged ? '' : f.contact,
    quota: Number(f.quota),
    plus_n_max: Number(f.plus),
    deadline: f.date ? zonedToInstant(f.date, f.time || '23:59', props.timezone).toISOString() : null,
    requires_approval: f.approval,
  })
}
</script>

<template>
  <form style="display:grid;gap:10px;padding:10px 0;" novalidate @submit.prevent="submit">
    <div class="form-grid">
      <label>
        <span class="section-lbl">SUBMITTER</span>
        <input :id="`${uid}-label`" v-model="f.label" class="hud-input" required maxlength="120" placeholder="Artist or promoter name">
      </label>
      <label v-if="!purged">
        <span class="section-lbl" style="display:inline-flex;align-items:center;gap:4px;"><Lock style="width:10px;height:10px;" aria-hidden="true" /> CONTACT · ENCRYPTED</span>
        <input v-model="f.contact" class="hud-input" maxlength="200" placeholder="Email or phone (optional)" autocomplete="off">
      </label>
      <label>
        <span class="section-lbl">QUOTA · HEADS</span>
        <input v-model.number="f.quota" class="hud-input" type="number" min="1" max="1000" inputmode="numeric">
      </label>
      <label>
        <span class="section-lbl">+N PER GUEST</span>
        <input v-model.number="f.plus" class="hud-input" type="number" min="0" :max="MAX_PLUS_N" inputmode="numeric">
      </label>
      <label>
        <span class="section-lbl">DEADLINE · {{ timezone.toUpperCase() }}</span>
        <span style="display:flex;gap:6px;">
          <input v-model="f.date" class="hud-input" type="date" aria-label="Deadline date">
          <input v-model="f.time" class="hud-input" type="time" aria-label="Deadline time" style="max-width:120px;">
        </span>
      </label>
      <label style="display:flex;align-items:center;gap:8px;min-height:44px;font-size:13px;align-self:end;">
        <input v-model="f.approval" type="checkbox"> NEEDS APPROVAL · new names arrive as pending
      </label>
    </div>
    <p style="margin:0;font-size:11px;color:var(--color-on-surface-variant);">A guest with +1 uses 2 heads. Waitlisted and declined guests use none.</p>
    <p v-if="errorText" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ errorText }}</p>
    <div style="display:flex;flex-wrap:wrap;gap:6px;">
      <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="saving || !f.label.trim()">{{ initial ? 'SAVE ALLOCATION' : 'ADD ALLOCATION' }}</button>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="emit('cancel')">CANCEL</button>
    </div>
  </form>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
}
@media (min-width: 640px) {
  .form-grid { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
label { display: grid; gap: 4px; }
</style>
