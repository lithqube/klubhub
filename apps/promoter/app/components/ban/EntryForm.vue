<script setup lang="ts">
import type { ApiError } from '~/types/event'
import type { BanEntry, BanPlain } from '~/types/sealed'
import {
  type BanField, customExpiry, EXPIRY_PRESETS, type ExpiryPreset, maxExpiryDate, minExpiryDate, NAME_MAX, NOTE_MAX, presetExpiry, REASON_MAX,
  validatePlain,
} from '~/utils/sealed/ban'

/**
 * Add or edit a ban list entry: name and reason required, email and note
 * optional, expiry 30 days / 6 months / 1 year / a custom date up to 3
 * years. Everything is encrypted in the browser before it is sent.
 * Emits the plain entry and the expiry; the page saves it.
 */
const props = defineProps<{ entry?: BanEntry | null, busy: boolean, serverError?: ApiError | null }>()
const emit = defineEmits<{ save: [plain: BanPlain, expiresAt: string], cancel: [] }>()
const uid = useId()

const editing = computed(() => !!props.entry)
const form = reactive({
  name: props.entry?.plain?.name ?? '',
  reason: props.entry?.plain?.reason ?? '',
  email: props.entry?.plain?.email ?? '',
  note: props.entry?.plain?.note ?? '',
})
// Editing keeps the current expiry unless another one is picked.
const preset = ref<ExpiryPreset | 'keep'>(props.entry ? 'keep' : '6m')
const custom = ref(props.entry ? props.entry.expires_at.slice(0, 10) : '')
const submitted = ref(false)
const nameEl = ref<HTMLInputElement | null>(null)
const customEl = ref<HTMLInputElement | null>(null)
onMounted(() => nameEl.value?.focus())

const minDate = computed(() => minExpiryDate())
const maxDate = computed(() => maxExpiryDate())
const fieldErrors = computed<Partial<Record<BanField, string>>>(() => {
  const e = validatePlain(form)
  if (preset.value === 'custom') {
    const c = customExpiry(custom.value)
    if (c.error) e.expires_at = c.error
  }
  return e
})
const shown = (f: BanField) => (submitted.value ? fieldErrors.value[f] : undefined)
const serverField = computed(() => (props.serverError?.error === 'invalid' ? props.serverError.field : undefined))

watch(preset, (p) => {
  if (p === 'custom') nextTick(() => customEl.value?.focus())
})

function expiresAt(): string | null {
  if (preset.value === 'keep') return props.entry?.expires_at ?? null
  if (preset.value === 'custom') return customExpiry(custom.value).at
  return presetExpiry(preset.value)
}

function submit() {
  submitted.value = true
  const errs = fieldErrors.value
  const first = (['name', 'reason', 'email', 'note', 'expires_at'] as BanField[]).find(f => errs[f])
  if (first) {
    nextTick(() => document.getElementById(`${uid}-${first}`)?.focus())
    return
  }
  const at = expiresAt()
  if (!at) return
  emit('save', { name: form.name, reason: form.reason, email: form.email, note: form.note }, at)
}
</script>

<template>
  <form class="glass panel" novalidate :aria-labelledby="`${uid}-h`" data-testid="ban-form" @submit.prevent="submit" @keydown.esc="emit('cancel')">
    <h2 :id="`${uid}-h`" class="lbl">{{ editing ? 'EDIT ENTRY' : 'ADD TO THE BAN LIST' }}</h2>
    <div class="grid">
      <div class="f">
        <label :for="`${uid}-name`" class="lbl">NAME <span class="req">REQUIRED</span></label>
        <input
          :id="`${uid}-name`" ref="nameEl" v-model="form.name" class="hud-input" :maxlength="NAME_MAX" autocomplete="off" autocapitalize="words"
          :aria-invalid="!!shown('name')" :aria-describedby="shown('name') ? `${uid}-name-e` : `${uid}-name-h`" :disabled="busy" data-testid="ban-name"
        >
        <span v-if="shown('name')" :id="`${uid}-name-e`" class="err">{{ shown('name') }}</span>
        <span v-else :id="`${uid}-name-h`" class="hint">As the person gives it at the door. The door matches names loosely and asks a manager.</span>
      </div>
      <div class="f">
        <label :for="`${uid}-email`" class="lbl">EMAIL <span class="opt">OPTIONAL</span></label>
        <input
          :id="`${uid}-email`" v-model="form.email" class="hud-input" type="email" autocomplete="off" :disabled="busy" :aria-invalid="!!shown('email')"
          :aria-describedby="shown('email') ? `${uid}-email-e` : undefined" data-testid="ban-email"
        >
        <span v-if="shown('email')" :id="`${uid}-email-e`" class="err">{{ shown('email') }}</span>
      </div>
    </div>
    <div class="f">
      <label :for="`${uid}-reason`" class="lbl">REASON <span class="req">REQUIRED</span></label>
      <textarea
        :id="`${uid}-reason`" v-model="form.reason" class="hud-input area" rows="2" :maxlength="REASON_MAX" :disabled="busy" :aria-invalid="!!shown('reason')"
        :aria-describedby="shown('reason') ? `${uid}-reason-e` : `${uid}-reason-h`" data-testid="ban-reason"
      />
      <span v-if="shown('reason')" :id="`${uid}-reason-e`" class="err">{{ shown('reason') }}</span>
      <span v-else :id="`${uid}-reason-h`" class="hint">Facts, not opinions: a manager reads it at the door with the manager PIN.</span>
    </div>
    <div class="f">
      <label :for="`${uid}-note`" class="lbl">NOTE <span class="opt">OPTIONAL</span></label>
      <textarea :id="`${uid}-note`" v-model="form.note" class="hud-input area" rows="2" :maxlength="NOTE_MAX" :disabled="busy" data-testid="ban-note" />
    </div>
    <fieldset class="f exp" :disabled="busy" :aria-describedby="shown('expires_at') ? `${uid}-expires_at-e` : undefined">
      <legend class="lbl">EXPIRES AFTER</legend>
      <div class="chips">
        <label v-if="editing" class="chip" :class="{ on: preset === 'keep' }">
          <input v-model="preset" type="radio" :name="`${uid}-exp`" value="keep" class="sr-only">
          KEEP {{ entry!.expires_at.slice(0, 10) }}
        </label>
        <label v-for="p in EXPIRY_PRESETS" :key="p.id" class="chip" :class="{ on: preset === p.id }">
          <input v-model="preset" type="radio" :name="`${uid}-exp`" :value="p.id" class="sr-only">
          {{ p.label }}
        </label>
        <label class="chip" :class="{ on: preset === 'custom' }">
          <input v-model="preset" type="radio" :name="`${uid}-exp`" value="custom" class="sr-only">
          CUSTOM
        </label>
      </div>
      <div v-if="preset === 'custom'" class="f">
        <label :for="`${uid}-expires_at`" class="lbl">UNTIL (AT MOST 3 YEARS)</label>
        <input
          :id="`${uid}-expires_at`" ref="customEl" v-model="custom" class="hud-input" type="date" :min="minDate" :max="maxDate" style="max-width:200px;"
          :aria-invalid="!!shown('expires_at')" data-testid="ban-custom-date"
        >
      </div>
      <span v-if="shown('expires_at') || serverField === 'expires_at'" :id="`${uid}-expires_at-e`" class="err">{{ shown('expires_at') ?? 'Pick an expiry between tomorrow and 3 years from now.' }}</span>
      <span v-else class="hint">Entries disappear by themselves when they expire.</span>
    </fieldset>
    <div class="row">
      <button type="button" class="btn-hud btn-hud-ghost act" :disabled="busy" @click="emit('cancel')">CANCEL</button>
      <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy" :aria-busy="busy" data-testid="ban-save">
        {{ busy ? 'ENCRYPTING…' : editing ? 'SAVE CHANGES' : 'ADD ENTRY' }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.panel { padding: 14px; display: grid; gap: 12px; min-width: 0; }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); }
.req, .opt { margin-left: 4px; font-size: 10px; color: var(--color-on-surface-variant); }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
.f { display: grid; gap: 4px; min-width: 0; }
.exp { border: 0; margin: 0; padding: 0; }
.hud-input { height: 44px; }
.area { height: auto; min-height: 64px; padding: 8px 12px; resize: vertical; }
.hint { font-size: 12px; color: var(--color-on-surface-variant); }
.err { font-size: 12px; color: var(--color-error); }
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.chip {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 12px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  color: var(--color-on-surface-variant);
  background: var(--color-surface-container);
  border: 1px dashed var(--color-outline);
  cursor: pointer;
}
.chip.on { color: var(--color-primary); border: 1px solid var(--color-primary-dim); }
.chip:focus-within { outline: 1px solid var(--color-primary); outline-offset: 2px; }
.row { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
.act { min-height: 44px; font-size: 11px; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
</style>
