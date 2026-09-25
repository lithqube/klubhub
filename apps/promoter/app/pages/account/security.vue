<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { renderSVG } from 'uqr'
import { useSessionStore } from '~/stores/session'

useHead({ title: 'Account security' })

const session = useSessionStore()
const { me } = storeToRefs(session)
const uri = ref<string | null>(null)
const code = ref('')
const busy = ref(false)
const error = ref<string | null>(null)

// Generated locally from our own otpauth:// URI; never sent to a QR service.
const qr = computed(() => (uri.value ? renderSVG(uri.value, { border: 1 }) : ''))
const secret = computed(() => (uri.value ? new URL(uri.value).searchParams.get('secret') ?? '' : ''))

async function start() {
  busy.value = true
  error.value = null
  try {
    uri.value = await session.enrollTotp()
  } catch {
    error.value = 'Could not start enrolment.'
  }
  busy.value = false
}

async function confirm() {
  busy.value = true
  const e = await session.confirmTotp(code.value.trim())
  busy.value = false
  if (e) {
    error.value = e === 'invalid_credentials' ? 'That code did not match. Wait for the next one and try again.' : 'Could not confirm the code.'
    return
  }
  uri.value = null
  code.value = ''
}
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">ACCOUNT SECURITY</div>
        <div class="page-sub">TWO-FACTOR AUTHENTICATION</div>
      </div>
    </div>
    <div class="page-body">
      <section class="glass hud-card" style="padding:24px;max-width:560px;">
        <template v-if="me?.mfa">
          <div class="section-lbl">STATUS</div>
          <p style="margin:8px 0 0;font-size:14px;">Authenticator app active for this session.</p>
        </template>
        <template v-else-if="!uri">
          <p style="margin:0 0 14px;font-size:14px;">
            Owners, admins and finance need an authenticator app (TOTP) to manage members, security and money.
          </p>
          <button class="btn-hud btn-hud-cta" :disabled="busy" style="min-height:44px;" @click="start">SET UP AUTHENTICATOR</button>
        </template>
        <form v-else class="space-y-4" @submit.prevent="confirm">
          <p style="margin:0;font-size:14px;">Scan with your authenticator app, then enter the 6-digit code.</p>
          <!-- eslint-disable-next-line vue/no-v-html -- locally generated SVG from our own URI -->
          <div aria-label="QR code for your authenticator app" role="img" style="width:200px;background:#fff;padding:6px;" v-html="qr" />
          <details>
            <summary class="section-lbl" style="cursor:pointer;">CAN'T SCAN? ENTER THE KEY</summary>
            <code style="word-break:break-all;font-size:13px;">{{ secret }}</code>
          </details>
          <label class="block">
            <span class="section-lbl">CODE</span>
            <input v-model="code" class="hud-input" inputmode="numeric" maxlength="6" autocomplete="one-time-code" required>
          </label>
          <p v-if="error" role="alert" style="color:var(--color-error);font-size:13px;margin:0;">{{ error }}</p>
          <button class="btn-hud btn-hud-cta" type="submit" :disabled="busy" style="min-height:44px;">CONFIRM</button>
        </form>
      </section>
    </div>
  </div>
</template>
