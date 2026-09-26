<script setup lang="ts">
import { useSessionStore } from '~/stores/session'

definePageMeta({ public: true })
useHead({ title: 'Sign in' })

const route = useRoute()
const session = useSessionStore()

const email = ref('')
const password = ref('')
const totp = ref('')
const needTotp = ref(false)
const busy = ref(false)
const error = ref<string | null>(null)

const messages: Record<string, string> = {
  invalid_credentials: 'Email, password or code is not correct.',
  totp_required: 'Enter the 6-digit code from your authenticator app.',
  csrf: 'Your browser blocked a security check. Reload the page and try again.',
  network_error: 'The server could not be reached.',
}

async function submit() {
  busy.value = true
  error.value = null
  const code = await session.login(email.value, password.value, needTotp.value ? totp.value : undefined)
  busy.value = false
  if (code === 'totp_required') {
    needTotp.value = true
    if (totp.value) error.value = messages.invalid_credentials!
    return
  }
  if (code) {
    error.value = messages[code] ?? 'Sign-in failed.'
    return
  }
  const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') && !route.query.next.startsWith('//')
    ? route.query.next
    : '/'
  await navigateTo(next)
}
</script>

<template>
  <AuthCard title="SIGN IN" subtitle="Your collective's private workspace.">
    <form class="space-y-4" novalidate @submit.prevent="submit">
      <label class="block">
        <span class="section-lbl">EMAIL</span>
        <input v-model="email" class="hud-input" type="email" name="email" autocomplete="username" required :disabled="needTotp">
      </label>
      <label class="block">
        <span class="section-lbl">PASSWORD</span>
        <input v-model="password" class="hud-input" type="password" name="password" autocomplete="current-password" required :disabled="needTotp">
      </label>
      <label v-if="needTotp" class="block">
        <span class="section-lbl">AUTHENTICATOR CODE</span>
        <input
          v-model="totp" class="hud-input" name="totp" inputmode="numeric" pattern="[0-9]{6}" maxlength="6"
          autocomplete="one-time-code" required autofocus
        >
      </label>
      <p v-if="error" role="alert" style="color:var(--color-error);font-family:var(--font-data);font-size:13px;margin:0;">
        {{ error }}
      </p>
      <button class="btn-hud btn-hud-cta w-full" type="submit" :disabled="busy" style="min-height:44px;">
        {{ busy ? 'CHECKING…' : needTotp ? 'VERIFY' : 'SIGN IN' }}
      </button>
    </form>
  </AuthCard>
</template>
