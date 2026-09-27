<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { Eye, EyeOff } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useSealedStore } from '~/stores/sealed'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import { formatFingerprint, normaliseKit, PASSPHRASE_MIN, passphraseStrength, STRENGTH_TEXT } from '~/utils/sealed/keys'
import { sealedErrorText, securityBlock, securityRefusal } from '~/utils/sealedText'

/**
 * RECOVER WITH KIT (owners): the typed kit re-creates the recovery key,
 * which opens the collective key; a new sealed passphrase makes a new
 * personal key that replaces the old one and is given access. The kit keeps
 * working afterwards.
 */
const emit = defineEmits<{ done: [], cancel: [] }>()
const store = useSealedStore()
const { working, progress, org } = storeToRefs(store)
const { me } = storeToRefs(useSessionStore())
const uid = useId()
const now = useNow({ interval: 30_000 })

const kit = ref('')
const pass = ref('')
const again = ref('')
const show = ref(false)
const mismatch = computed(() => again.value !== '' && pass.value !== '' && again.value !== pass.value)
const error = ref<ApiError | null>(null)
const kitEl = ref<HTMLTextAreaElement | null>(null)
const busy = computed(() => working.value === 'recover')
const block = computed(() => securityBlock(me.value, now.value.getTime()))
const refusal = computed(() => securityRefusal(error.value))
const typed = computed(() => normaliseKit(kit.value).length)
const strength = computed(() => passphraseStrength(pass.value))

async function submit() {
  error.value = null
  if (pass.value.length < PASSPHRASE_MIN) {
    error.value = { error: 'passphrase_short' } as ApiError
    return
  }
  if (pass.value !== again.value) {
    error.value = { error: 'passphrase_mismatch' } as ApiError
    return
  }
  try {
    await store.recoverWithKit(kit.value, pass.value)
    kit.value = ''
    pass.value = ''
    again.value = ''
    emit('done')
  } catch (e) {
    error.value = e as ApiError
    if (error.value.error.startsWith('kit_')) nextTick(() => kitEl.value?.focus())
  }
}
</script>

<template>
  <form class="glass panel" novalidate :aria-labelledby="`${uid}-h`" data-testid="sealed-recover" @submit.prevent="submit">
    <h3 :id="`${uid}-h`" class="lbl">RECOVER WITH KIT</h3>
    <p class="txt">
      For owners who lost their passphrase. Type the 8 groups of your collective's recovery kit and choose a new sealed passphrase.
      <template v-if="org?.recovery_fingerprint">The kit's fingerprint must be <strong class="mono">{{ formatFingerprint(org.recovery_fingerprint) }}</strong>.</template>
    </p>
    <SettingsSealedSecurity v-if="block && !refusal" :kind="block" next="/settings#sealed" action="Recovering with the kit" />
    <SettingsSealedUsername />
    <div class="f">
      <label :for="`${uid}-k`" class="lbl">RECOVERY KIT</label>
      <textarea
        :id="`${uid}-k`" ref="kitEl" v-model="kit" class="hud-input kit" rows="2" autocomplete="off" autocapitalize="characters" spellcheck="false"
        :aria-describedby="`${uid}-kh`" :aria-invalid="!!error?.error.startsWith('kit_')" :disabled="busy" data-testid="sealed-recover-kit"
      />
      <span :id="`${uid}-kh`" class="hint">{{ typed }} of 56 characters · spaces, dashes and case don't matter</span>
    </div>
    <div class="two">
      <div class="f">
        <label :for="`${uid}-p`" class="lbl">NEW SEALED PASSPHRASE</label>
        <div class="pw">
          <input
            :id="`${uid}-p`" v-model="pass" class="hud-input" :type="show ? 'text' : 'password'" name="sealed-passphrase" autocomplete="new-password" :disabled="busy"
            :aria-describedby="`${uid}-nl ${uid}-ph`" data-testid="sealed-recover-passphrase"
          >
          <button type="button" class="btn-hud btn-hud-ghost eye" :aria-label="show ? 'Hide passphrase' : 'Show passphrase'" :aria-pressed="show" @click="show = !show">
            <EyeOff v-if="show" class="ic" aria-hidden="true" /><Eye v-else class="ic" aria-hidden="true" />
          </button>
        </div>
        <span :id="`${uid}-nl`" class="hint">Not your login password.</span>
        <span :id="`${uid}-ph`" class="hint" aria-live="polite">{{ pass ? STRENGTH_TEXT[strength] : `At least ${PASSPHRASE_MIN} characters.` }}</span>
      </div>
      <div class="f">
        <label :for="`${uid}-a`" class="lbl">TYPE IT AGAIN</label>
        <input
          :id="`${uid}-a`" v-model="again" class="hud-input" :type="show ? 'text' : 'password'" name="sealed-passphrase-again" autocomplete="new-password" :disabled="busy"
          :aria-invalid="mismatch" :aria-describedby="mismatch ? `${uid}-mm` : undefined" data-testid="sealed-recover-passphrase-again"
        >
        <span v-if="mismatch" :id="`${uid}-mm`" class="err">Not the same as the new passphrase.</span>
      </div>
    </div>
    <SettingsSealedProgress v-if="busy" label="RECOVERING… MAKING YOUR NEW KEY" :progress="progress" />
    <SettingsSealedSecurity v-if="refusal" :kind="refusal" next="/settings#sealed" action="Recovering with the kit" />
    <p v-else-if="error" role="alert" class="err" data-testid="sealed-recover-error">{{ sealedErrorText(error) }}</p>
    <div class="row">
      <button type="button" class="btn-hud btn-hud-ghost act" :disabled="busy" @click="emit('cancel')">CANCEL</button>
      <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy || !!block" :aria-busy="busy" data-testid="sealed-recover-submit">
        {{ busy ? 'RECOVERING…' : 'RECOVER' }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.hint { font-size: 12px; color: var(--color-on-surface-variant); }
.err { margin: 0; font-size: 12px; color: var(--color-error); }
.f { display: grid; gap: 4px; min-width: 0; }
.two { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px; }
.hud-input { height: 44px; }
.pw { display: flex; gap: 6px; min-width: 0; }
.pw .hud-input { flex: 1; min-width: 0; }
.eye { min-width: 44px; min-height: 44px; padding: 0; }
.ic { width: 18px; height: 18px; }
.kit { height: auto; min-height: 64px; padding: 8px 12px; resize: vertical; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 15px; letter-spacing: .06em; text-transform: uppercase; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.row { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
.act { min-height: 44px; font-size: 11px; }
</style>
