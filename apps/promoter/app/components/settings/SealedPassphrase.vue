<script setup lang="ts">
import { Eye, EyeOff } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useSealedStore } from '~/stores/sealed'
import type { ApiError } from '~/types/event'
import { PASSPHRASE_MIN, passphraseStrength, STRENGTH_TEXT } from '~/utils/sealed/keys'
import { sealedErrorText } from '~/utils/sealedText'

/**
 * SET UP YOUR KEY: a sealed passphrase (separate from the login password,
 * ≥ 12 characters, typed twice) protects this user's member key. The key
 * is made in this browser; only its public half and the encrypted private
 * half reach the server.
 */
const props = withDefaults(defineProps<{ step?: string }>(), { step: '' })
const emit = defineEmits<{ done: [] }>()
const store = useSealedStore()
const { working, progress } = storeToRefs(store)
const uid = useId()

const pass = ref('')
const again = ref('')
const show = ref(false)
const touched = ref(false)
const error = ref<ApiError | null>(null)
const passEl = ref<HTMLInputElement | null>(null)
const againEl = ref<HTMLInputElement | null>(null)

const strength = computed(() => passphraseStrength(pass.value))
const hint = computed(() => (pass.value ? STRENGTH_TEXT[strength.value] : `At least ${PASSPHRASE_MIN} characters.`))
const mismatch = computed(() => touched.value && again.value !== '' && again.value !== pass.value)
const busy = computed(() => working.value === 'create')

async function submit() {
  touched.value = true
  error.value = null
  if (pass.value.length < PASSPHRASE_MIN) {
    error.value = { error: 'passphrase_short' } as ApiError
    passEl.value?.focus()
    return
  }
  if (again.value !== pass.value) {
    error.value = { error: 'passphrase_mismatch' } as ApiError
    againEl.value?.focus()
    return
  }
  try {
    await store.setupMemberKey(pass.value)
    pass.value = ''
    again.value = ''
    emit('done')
  } catch (e) {
    error.value = e as ApiError
  }
}
</script>

<template>
  <form class="glass panel" novalidate :aria-labelledby="`${uid}-h`" data-testid="sealed-create" @submit.prevent="submit">
    <h3 :id="`${uid}-h`" class="lbl">SET UP YOUR KEY<template v-if="props.step"> · {{ props.step }}</template></h3>
    <SettingsSealedUsername />
    <p class="txt">
      Sealed data (the ban list) is encrypted in your browser. Your personal key is protected by a <strong>sealed passphrase</strong>:
      separate from your login password, never sent to the server. Nobody can reset it for you, so pick one you will remember.
    </p>
    <div class="f">
      <label :for="`${uid}-p`" class="lbl">SEALED PASSPHRASE</label>
      <div class="pw">
        <input
          :id="`${uid}-p`" ref="passEl" v-model="pass" class="hud-input" :type="show ? 'text' : 'password'" name="sealed-passphrase" autocomplete="new-password"
          :aria-describedby="`${uid}-nl ${uid}-hint`" :aria-invalid="error?.error === 'passphrase_short'" :disabled="busy" data-testid="sealed-new-passphrase"
        >
        <button type="button" class="btn-hud btn-hud-ghost eye" :aria-label="show ? 'Hide passphrase' : 'Show passphrase'" :aria-pressed="show" @click="show = !show">
          <EyeOff v-if="show" class="ic" aria-hidden="true" /><Eye v-else class="ic" aria-hidden="true" />
        </button>
      </div>
      <span :id="`${uid}-nl`" class="hint">Not your login password.</span>
      <span :id="`${uid}-hint`" class="hint" :class="`s-${pass ? strength : 'none'}`" aria-live="polite">{{ hint }}</span>
    </div>
    <div class="f">
      <label :for="`${uid}-a`" class="lbl">TYPE IT AGAIN</label>
      <input
        :id="`${uid}-a`" ref="againEl" v-model="again" class="hud-input" :type="show ? 'text' : 'password'" name="sealed-passphrase-again" autocomplete="new-password"
        :aria-invalid="mismatch" :aria-describedby="mismatch ? `${uid}-mm` : undefined" :disabled="busy" data-testid="sealed-new-passphrase-again" @blur="touched = true"
      >
      <span v-if="mismatch" :id="`${uid}-mm`" class="err" data-testid="sealed-passphrase-mismatch">Not the same as above.</span>
    </div>
    <SettingsSealedProgress v-if="busy" label="MAKING YOUR KEY… THIS TAKES A FEW SECONDS" :progress="progress" />
    <p v-if="error" role="alert" class="err" data-testid="sealed-create-error">{{ sealedErrorText(error) }}</p>
    <div class="row">
      <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy" :aria-busy="busy">{{ busy ? 'MAKING KEY…' : 'CREATE MY KEY' }}</button>
    </div>
  </form>
</template>

<style scoped>
.hud-input { height: 44px; }
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.f { display: grid; gap: 4px; min-width: 0; }
.pw { display: flex; gap: 6px; min-width: 0; }
.pw .hud-input { flex: 1; min-width: 0; }
.eye { min-width: 44px; min-height: 44px; padding: 0; }
.ic { width: 18px; height: 18px; }
.hint { font-size: 12px; color: var(--color-on-surface-variant); }
.hint.s-short, .hint.s-weak { color: var(--color-status-archived); }
.hint.s-strong { color: var(--color-primary); }
.err { margin: 0; font-size: 12px; color: var(--color-error); }
.row { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
.act { min-height: 44px; font-size: 11px; }
</style>
