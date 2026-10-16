<script setup lang="ts">
import { useBookingsStore } from '~/stores/bookings'
import type { ApiError } from '~/types/event'
import type { TalentContact, TalentContactInput, TalentSource } from '~/types/bookings'
import { TALENT_SOURCES } from '~/types/bookings'

/**
 * Talent contact create/edit form (04.10-05). A contact needs a name or an
 * email; without an email it stays matchable but never sendable (OFFER-10).
 */
const props = defineProps<{ contact: TalentContact | null }>()
const emit = defineEmits<{ saved: [], cancel: [] }>()

const store = useBookingsStore()

const saving = ref(false)
const error = ref<ApiError | null>(null)
const form = reactive({ name: '', email: '', phone: '', city: '', country: '', genres: '', source: 'manual' as TalentSource, notes: '' })

watch(() => props.contact?.id, () => {
  error.value = null
  const c = props.contact
  form.name = c?.name ?? ''
  form.email = c?.email ?? ''
  form.phone = c?.phone ?? ''
  form.city = c?.city ?? ''
  form.country = c?.country ?? ''
  form.genres = c?.genres.join(', ') ?? ''
  form.source = c?.source ?? 'manual'
  form.notes = c?.notes ?? ''
}, { immediate: true })

const canSubmit = computed(() => !saving.value && !!(form.name.trim() || form.email.trim()))

async function submit() {
  saving.value = true
  error.value = null
  const input: TalentContactInput = {
    name: form.name.trim(),
    email: form.email.trim(),
    phone: form.phone.trim(),
    city: form.city.trim(),
    country: form.country.trim(),
    genres: form.genres.split(',').map(g => g.trim()).filter(Boolean),
    source: form.source,
    notes: form.notes.trim(),
  }
  try {
    if (props.contact) await store.updateTalent(props.contact.id, input)
    else await store.createTalent(input)
    emit('saved')
  } catch (e) {
    error.value = e as ApiError
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <form class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" data-testid="talent-form" novalidate @submit.prevent="submit">
    <h3 class="section-lbl" style="margin:0;">{{ contact ? 'EDIT DJ' : 'ADD DJ' }}</h3>
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:8px;">
      <label style="display:grid;gap:2px;font-size:11px;">
        NAME
        <input v-model="form.name" class="hud-input" style="min-height:40px;" data-testid="talent-name">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        EMAIL
        <input v-model="form.email" type="email" class="hud-input" style="min-height:40px;" data-testid="talent-email">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        PHONE
        <input v-model="form.phone" class="hud-input" style="min-height:40px;" data-testid="talent-phone">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        CITY
        <input v-model="form.city" class="hud-input" style="min-height:40px;" data-testid="talent-city">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        COUNTRY
        <input v-model="form.country" class="hud-input" placeholder="DE" maxlength="2" style="min-height:40px;text-transform:uppercase;" data-testid="talent-country">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        GENRES (COMMA SEPARATED)
        <input v-model="form.genres" class="hud-input" placeholder="techno, house" style="min-height:40px;" data-testid="talent-genres">
      </label>
      <label style="display:grid;gap:2px;font-size:11px;">
        SOURCE
        <select v-model="form.source" class="hud-input" style="min-height:40px;" data-testid="talent-source">
          <option v-for="s in TALENT_SOURCES" :key="s" :value="s">{{ s.toUpperCase().replace('_', ' ') }}</option>
        </select>
      </label>
    </div>
    <label style="display:grid;gap:2px;font-size:11px;">
      NOTES
      <textarea v-model="form.notes" class="hud-input" rows="2" style="min-height:60px;" data-testid="talent-notes"/>
    </label>
    <p v-if="error" role="alert" class="glass" style="padding:10px 12px;border-left:3px solid var(--color-error);font-size:13px;">
      {{ error.error === 'duplicate' ? 'A contact with this email already exists.' : (error.error === 'invalid' ? `Invalid ${error.field ?? 'input'}: ${error.problem ?? ''}` : 'Could not save the contact.') }}
    </p>
    <div style="display:flex;gap:8px;">
      <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;" :disabled="!canSubmit" data-testid="talent-save">SAVE DJ</button>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="emit('cancel')">CANCEL</button>
    </div>
  </form>
</template>
