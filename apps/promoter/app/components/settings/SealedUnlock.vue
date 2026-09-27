<script setup lang="ts">
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
    pass.value = ''
    nextTick(() => input.value?.focus())
  }
}
</script>

<template>
  <form class="glass panel" novalidate :aria-labelledby="`${uid}-h`" data-testid="sealed-unlock" @submit.prevent="submit">
    <h3 :id="`${uid}-h`" class="lbl">{{ props.title }}</h3>
    <p v-if="lockNote" class="note" data-testid="sealed-lock-note">{{ lockNote }}</p>
    <p v-if="props.why" class="txt">{{ props.why }}</p>
    <div class="line">
      <label :for="`${uid}-p`" class="sr-only">Sealed passphrase</label>
      <input
        :id="`${uid}-p`" ref="input" v-model="pass" class="hud-input" type="password" autocomplete="current-password" placeholder="Sealed passphrase"
        :disabled="busy" :aria-invalid="!!error" :aria-describedby="error ? `${uid}-e` : undefined" data-testid="sealed-passphrase"
      >
      <button type="submit" class="btn-hud btn-hud-cta act" :disabled="busy" :aria-busy="busy">{{ busy ? 'UNLOCKING…' : 'UNLOCK' }}</button>
    </div>
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
.line .hud-input { flex: 1 1 200px; min-width: 0; }
.act { min-height: 44px; font-size: 11px; }
.err { margin: 0; font-size: 12px; color: var(--color-error); }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
</style>
