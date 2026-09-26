<script setup lang="ts">
import { useSessionStore } from '~/stores/session'

definePageMeta({ public: true })
useHead({ title: 'Set your password' })

const session = useSessionStore()
const token = ref('')
const password = ref('')
const confirm = ref('')
const busy = ref(false)
const error = ref<string | null>(null)
const done = ref(false)

// The one-time token arrives in the URL fragment (never sent to servers or
// in Referer headers); read it on the client and drop it from the address bar.
onMounted(() => {
  const params = new URLSearchParams(window.location.hash.slice(1))
  token.value = params.get('token') ?? ''
  history.replaceState(null, '', window.location.pathname)
})

const tooShort = computed(() => password.value.length > 0 && password.value.length < 12)
const mismatch = computed(() => confirm.value.length > 0 && confirm.value !== password.value)

const messages: Record<string, string> = {
  invalid_link: 'This link is invalid, already used or expired. Ask for a new one.',
  weak_password: 'Use at least 12 characters. A few random words work well.',
}

async function submit() {
  if (tooShort.value || mismatch.value || !token.value) return
  busy.value = true
  error.value = await session.completeSetup(token.value, password.value)
  busy.value = false
  if (error.value) {
    error.value = messages[error.value] ?? 'Could not set the password.'
    return
  }
  done.value = true
}
</script>

<template>
  <AuthCard title="SET YOUR PASSWORD" subtitle="One-time setup link.">
    <div v-if="done" class="space-y-4">
      <p style="font-family:var(--font-data);font-size:14px;margin:0;">Password set. Sign in, then add an authenticator app under Account security.</p>
      <NuxtLink to="/login" class="btn-hud btn-hud-cta w-full" style="min-height:44px;">GO TO SIGN IN</NuxtLink>
    </div>
    <form v-else class="space-y-4" novalidate @submit.prevent="submit">
      <p v-if="!token" role="alert" style="color:var(--color-error);font-size:13px;margin:0;">
        This page needs the link from your invitation or setup message.
      </p>
      <label class="block">
        <span class="section-lbl">NEW PASSWORD</span>
        <input v-model="password" class="hud-input" type="password" autocomplete="new-password" minlength="12" required :aria-invalid="tooShort">
        <span v-if="tooShort" style="font-size:12px;color:var(--color-error);">At least 12 characters.</span>
      </label>
      <label class="block">
        <span class="section-lbl">REPEAT PASSWORD</span>
        <input v-model="confirm" class="hud-input" type="password" autocomplete="new-password" required :aria-invalid="mismatch">
        <span v-if="mismatch" style="font-size:12px;color:var(--color-error);">Passwords do not match.</span>
      </label>
      <p v-if="error" role="alert" style="color:var(--color-error);font-size:13px;margin:0;">{{ error }}</p>
      <button class="btn-hud btn-hud-cta w-full" type="submit" :disabled="busy || !token" style="min-height:44px;">
        {{ busy ? 'SAVING…' : 'SET PASSWORD' }}
      </button>
    </form>
  </AuthCard>
</template>
