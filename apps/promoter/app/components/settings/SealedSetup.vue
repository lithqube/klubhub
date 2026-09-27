<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { Download } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useOrgStore } from '~/stores/org'
import { useSealedStore } from '~/stores/sealed'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import { formatFingerprint } from '~/utils/sealed/keys'
import { sealedErrorText, securityBlock, securityRefusal } from '~/utils/sealedText'

/**
 * SET UP SEALED DATA (owners): a new collective key (OSK) and a mandatory
 * recovery kit. The kit is shown once; the owner downloads or writes it
 * down, then re-types two groups picked at random — setup cannot finish
 * without that. Nothing reaches the server before FINISH; leaving the
 * wizard discards the kit (a new one is made next time).
 */
const emit = defineEmits<{ done: [] }>()
const store = useSealedStore()
const { setup, working } = storeToRefs(store)
const { me } = storeToRefs(useSessionStore())
const { org } = storeToRefs(useOrgStore())
const uid = useId()
const now = useNow({ interval: 30_000 })

type Step = 'intro' | 'kit' | 'check'
const step = ref<Step>(setup.value ? 'kit' : 'intro')
const downloaded = ref(false)
const wroteDown = ref(false)
const answers = ref<[string, string]>(['', ''])
const error = ref<ApiError | null>(null)
const heading = ref<HTMLElement | null>(null)
const answerEls = ref<HTMLInputElement[]>([])
const focusFirst = () => nextTick(() => answerEls.value[0]?.focus())
const busy = computed(() => working.value === 'setup')
const block = computed(() => securityBlock(me.value, now.value.getTime()))
const refusal = computed(() => securityRefusal(error.value))
const next = '/settings#sealed'

// Each step takes focus on its heading (the check step on its first field).
watch(step, (s) => {
  if (s !== 'check') nextTick(() => heading.value?.focus())
})
onBeforeUnmount(() => store.cancelSetup())

async function start() {
  error.value = null
  await store.startSetup()
  downloaded.value = false
  wroteDown.value = false
  answers.value = ['', '']
  step.value = 'kit'
}

function download() {
  const text = store.kitText(org.value?.name ?? 'Your collective')
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url
  a.download = `klubhub-recovery-kit-${org.value?.slug ?? 'collective'}.txt`
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
  downloaded.value = true
}

function toCheck() {
  answers.value = ['', '']
  error.value = null
  step.value = 'check'
  focusFirst()
}

function cancel() {
  if (!window.confirm('Stop the setup? This kit is thrown away; you\'ll get a new one when you start again.')) return
  store.cancelSetup()
  error.value = null
  step.value = 'intro'
}

async function finish() {
  error.value = null
  try {
    await store.finishSetup(answers.value)
    emit('done')
  } catch (e) {
    error.value = e as ApiError
    if (error.value.error === 'kit_mismatch') focusFirst()
  }
}

const groupNo = (i: number) => i + 1
</script>

<template>
  <section class="glass panel" :aria-labelledby="`${uid}-h`" data-testid="sealed-setup">
    <template v-if="step === 'intro'">
      <h3 :id="`${uid}-h`" ref="heading" tabindex="-1" class="lbl">SET UP SEALED DATA · STEP 1 OF 3</h3>
      <p class="txt">
        This creates your collective's key for sealed data. The ban list is encrypted with it in the browser; the server only stores ciphertext
        and can't read it. You then give access to other members and door devices.
      </p>
      <p class="txt">
        You also get a <strong>recovery kit</strong>: the only way back in if every owner forgets their passphrase. It is shown once. Keep it offline.
      </p>
      <SettingsSealedSecurity v-if="block" :kind="block" :next="next" action="Setting up sealed data" />
      <div class="row">
        <button type="button" class="btn-hud btn-hud-cta act" :disabled="!!block" data-testid="sealed-setup-start" @click="start">START SETUP</button>
      </div>
    </template>

    <template v-else-if="step === 'kit' && setup">
      <h3 :id="`${uid}-h`" ref="heading" tabindex="-1" class="lbl">RECOVERY KIT · STEP 2 OF 3 · SHOWN ONCE</h3>
      <p class="txt">
        Download it or write it down now, then store it offline (printed, or in a password manager). Anyone with it can read the ban list.
        KlubHub cannot show it again or recover it for you.
      </p>
      <ol class="kit" aria-label="Recovery kit groups" data-testid="sealed-kit">
        <li v-for="(g, i) in setup.groups" :key="i" :data-testid="`kit-group-${groupNo(i)}`">
          <span class="no" aria-hidden="true">{{ groupNo(i) }}</span>
          <span class="g" :aria-label="`Group ${groupNo(i)}: ${g.split('').join(' ')}`">{{ g }}</span>
        </li>
      </ol>
      <p class="fp">FINGERPRINT <span data-testid="sealed-kit-fingerprint">{{ formatFingerprint(setup.fingerprint) }}</span></p>
      <div class="row start">
        <button type="button" class="btn-hud btn-hud-ghost act" data-testid="sealed-kit-download" @click="download">
          <Download class="ic" aria-hidden="true" /> {{ downloaded ? 'DOWNLOADED · AGAIN' : 'DOWNLOAD KIT' }}
        </button>
        <label class="ack">
          <input v-model="wroteDown" type="checkbox" data-testid="sealed-kit-written">
          I printed it or wrote it down
        </label>
      </div>
      <div class="row">
        <button type="button" class="btn-hud btn-hud-ghost act" @click="cancel">CANCEL</button>
        <button type="button" class="btn-hud btn-hud-cta act" :disabled="!downloaded && !wroteDown" data-testid="sealed-kit-continue" @click="toCheck">
          CONTINUE
        </button>
      </div>
      <p v-if="!downloaded && !wroteDown" class="hint">Download the kit or tick the box to continue.</p>
    </template>

    <form v-else-if="step === 'check' && setup" novalidate class="form" @submit.prevent="finish">
      <h3 :id="`${uid}-h`" ref="heading" tabindex="-1" class="lbl">CHECK THE KIT · STEP 3 OF 3</h3>
      <p class="txt">From your stored kit (not from memory of this screen), type these two groups. Case, spaces and dashes don't matter.</p>
      <div class="answers">
        <div v-for="(pos, k) in setup.challenge" :key="pos" class="f">
          <label :for="`${uid}-g${k}`" class="lbl">GROUP {{ groupNo(pos) }}</label>
          <input
            :id="`${uid}-g${k}`" ref="answerEls" v-model="answers[k]" class="hud-input mono"
            autocomplete="off" autocapitalize="characters" spellcheck="false" maxlength="12" :disabled="busy" :aria-invalid="error?.error === 'kit_mismatch'"
            :data-testid="`sealed-check-${k}`" :data-group="groupNo(pos)"
          >
        </div>
      </div>
      <SettingsSealedSecurity v-if="refusal" :kind="refusal" :next="next" action="Setting up sealed data" />
      <p v-else-if="error" role="alert" class="err" data-testid="sealed-setup-error">
        {{ sealedErrorText(error) }}
      </p>
      <p v-if="refusal" class="hint">After signing in again, start the setup again: you'll get a new kit (this one is not saved).</p>
      <div class="row">
        <button type="button" class="btn-hud btn-hud-ghost act" :disabled="busy" @click="step = 'kit'">SHOW THE KIT AGAIN</button>
        <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy || !answers[0].trim() || !answers[1].trim()" :aria-busy="busy" data-testid="sealed-setup-finish">
          {{ busy ? 'SETTING UP…' : 'FINISH SETUP' }}
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.form { display: grid; gap: 10px; min-width: 0; }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); }
h3.lbl:focus { outline: none; }
h3.lbl:focus-visible { outline: 1px solid var(--color-primary); outline-offset: 2px; }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.hint { margin: 0; font-size: 12px; color: var(--color-on-surface-variant); }
.err { margin: 0; font-size: 12px; color: var(--color-error); }
.row { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; align-items: center; }
.row.start { justify-content: flex-start; }
.act { min-height: 44px; font-size: 11px; }
.ic { width: 16px; height: 16px; }
.kit {
  list-style: none;
  margin: 0;
  padding: 12px;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
  background: var(--color-surface-container);
  border: 1px dashed var(--color-outline);
}
@media (min-width: 640px) {
  .kit { grid-template-columns: repeat(4, minmax(0, 1fr)); }
}
.kit li { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
.no { font-family: var(--font-terminal); font-size: 11px; color: var(--color-on-surface-variant); min-width: 12px; }
.g {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: .08em;
  color: var(--color-primary);
}
.fp { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .06em; color: var(--color-on-surface-variant); overflow-wrap: anywhere; }
.ack { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; font-size: 13px; cursor: pointer; }
.ack input { width: 18px; height: 18px; }
.answers { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px; }
.f { display: grid; gap: 4px; min-width: 0; }
.hud-input { height: 44px; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 16px; letter-spacing: .08em; text-transform: uppercase; }
</style>
