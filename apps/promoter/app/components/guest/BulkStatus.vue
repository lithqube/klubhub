<script setup lang="ts">
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { BulkResult, GuestStatus } from '~/types/guest'
import { guestErrorText, parseEmails, STATUS_LABEL, STATUSES } from '~/utils/guests'

/**
 * Bulk status by pasting emails (e.g. "who confirmed" from a mail thread).
 * Matching is exact, through the server's email blind index; emails that
 * match nobody are listed back so nothing silently disappears.
 */
const emit = defineEmits<{ done: [message: string], cancel: [] }>()
const store = useGuestStore()
const uid = useId()

const text = ref('')
const status = ref<GuestStatus>('going')
const result = ref<BulkResult | null>(null)
const error = ref<ApiError | null>(null)
const saving = ref(false)
const emails = computed(() => parseEmails(text.value))

async function submit() {
  if (!emails.value.length) return
  saving.value = true
  error.value = null
  result.value = null
  try {
    const r = await store.bulkStatus({ status: status.value, emails: emails.value })
    result.value = r
    if (!r.unmatched.length) {
      text.value = ''
      emit('done', `${r.updated} set to ${STATUS_LABEL[status.value]} (${r.matched} matched).`)
    }
  } catch (e) {
    error.value = e as ApiError
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <form class="hud-card" style="padding:12px 14px;display:grid;gap:10px;" aria-labelledby="bulk-h" novalidate @submit.prevent="submit">
    <h3 id="bulk-h" class="section-lbl" style="margin:0;">SET STATUS BY EMAIL</h3>
    <label style="display:grid;gap:4px;">
      <span style="font-size:12px;color:var(--color-on-surface-variant);">Paste emails in any layout — a column, a mail thread, a CSV row. Only lists that collect contacts have emails.</span>
      <textarea :id="`${uid}-emails`" v-model="text" class="hud-textarea" rows="4" placeholder="aiko@label.example, rafael@press.example" />
    </label>
    <div style="display:flex;flex-wrap:wrap;align-items:end;gap:8px;">
      <div style="display:grid;gap:4px;">
        <label :for="`${uid}-status`" class="section-lbl">NEW STATUS</label>
        <select :id="`${uid}-status`" v-model="status" class="hud-input" style="min-width:160px;">
          <option v-for="s in STATUSES" :key="s" :value="s">{{ STATUS_LABEL[s] }}</option>
        </select>
      </div>
      <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="saving || !emails.length">
        {{ emails.length ? `APPLY TO ${emails.length} ${emails.length === 1 ? 'EMAIL' : 'EMAILS'}` : 'APPLY' }}
      </button>
      <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('cancel')">CLOSE</button>
    </div>
    <div v-if="result" role="status" style="font-size:13px;">
      {{ result.matched }} matched · {{ result.updated }} changed.
      <template v-if="result.unmatched.length">
        <strong style="color:var(--color-status-archived);">{{ result.unmatched.length }} not found:</strong>
        <ul style="margin:4px 0 0;padding-left:18px;">
          <li v-for="e in result.unmatched" :key="e" style="font-family:var(--font-command);font-size:12px;">{{ e }}</li>
        </ul>
      </template>
    </div>
    <p v-if="error" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ guestErrorText(error) }}</p>
  </form>
</template>
