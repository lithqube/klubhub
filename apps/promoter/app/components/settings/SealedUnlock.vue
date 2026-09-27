<script setup lang="ts">
import { Eye, EyeOff } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useSealedStore } from '~/stores/sealed'
import type { ApiError } from '~/types/event'
import { sealedErrorText } from '~/utils/sealedText'

/**
 * UNLOCK: the sealed passphrase opens this user's key and the collective
 * key in memory (never stored); they lock again after 30 minutes idle, on
 * LOCK and on sign-out. Used in settings, on the ban list and on the door tab.
 */
const props = withDefaults(defineProps<{ title?: string, why?: string }>(), { title: 'UNLOCK', why: '' })
const emit = defineEmits<{ unlocked: [] }>()
const store = useSealedStore()
const { working, progress, lockedBy } = storeToRefs(store)
const uid = useId()

const pass = ref('')
const show = ref(false)
const error = ref<ApiError | null>(null)
const input = ref<HTMLInputElement | null>(null)
const busy = computed(() => working.value === 'unlock')
const lockNote = computed(() => (lockedBy.value === 'idle' ? 'Locked after 30 minutes without activity.' : lockedBy.value === 'signout' ? 'Locked when you signed out.' : ''))

async function submit() {
  error.value = null
  if (!pass.value) {
    input.value?.focus()
    return
  }
  try {
    await store.unlock(pass.value)
    pass.value = ''
    emit('unlocked')
  } catch (e) {
    error.value = e as ApiError
    // Keep what was typed and select it: fixing one typo beats typing it all again.
    nextTick(() => {
      input.value?.focus()
      input.value?.select()
    })
  }
}
</script>

<template>
  <form class="glass panel" novalidate :aria-labelledby="`${uid}-h`" data-testid="sealed-unlock" @submit.prevent="submit">
    <h3 :id="`${uid}-h`" class="lbl">{{ props.title }}</h3>
    <p v-if="lockNote" class="note" data-testid="sealed-lock-note">{{ lockNote }}</p>
    <p v-if="props.why" class="txt">{{ props.why }}</p>
    <SettingsSealedUsername />
    <label :for="`${uid}-p`" class="lbl">SEALED PASSPHRASE</label>
    <div class="line">
      <div class="pw">
        <input
          :id="`${uid}-p`" ref="input" v-model="pass" class="hud-input" :type="show ? 'text' : 'password'" name="sealed-passphrase" autocomplete="current-password"
          :disabled="busy" :aria-invalid="!!error" :aria-describedby="error ? `${uid}-e ${uid}-nl` : `${uid}-nl`" data-testid="sealed-passphrase"
        >
        <button type="button" class="btn-hud btn-hud-ghost eye" :aria-label="show ? 'Hide passphrase' : 'Show passphrase'" :aria-pressed="show" @click="show = !show">
          <EyeOff v-if="show" class="ic" aria-hidden="true" /><Eye v-else class="ic" aria-hidden="true" />
        </button>
      </div>
      <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy" :aria-busy="busy">{{ busy ? 'UNLOCKING…' : 'UNLOCK' }}</button>
    </div>
    <span :id="`${uid}-nl`" class="hint">Not your login password.</span>
    <SettingsSealedProgress v-if="busy" label="UNLOCKING… THIS TAKES A FEW SECONDS" :progress="progress" />
    <p v-if="error" :id="`${uid}-e`" role="alert" class="err" data-testid="sealed-unlock-error">{{ sealedErrorText(error) }}</p>
    <slot />
  </form>
</template>

<style scoped>
.hud-input { height: 44px; }
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.note { margin: 0; font-size: 13px; color: var(--color-on-surface); }
.line { display: flex; flex-wrap: wrap; gap: 8px; min-width: 0; }
.pw { display: flex; gap: 6px; flex: 1 1 200px; min-width: 0; }
.pw .hud-input { flex: 1; min-width: 0; }
.eye { min-width: 44px; min-height: 44px; padding: 0; }
.ic { width: 18px; height: 18px; }
.hint { margin: 0; font-size: 12px; color: var(--color-on-surface-variant); }
.act { min-height: 44px; font-size: 11px; }
.err { margin: 0; font-size: 12px; color: var(--color-error); }
</style>
