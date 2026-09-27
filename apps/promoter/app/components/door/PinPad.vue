<script setup lang="ts">
import { Delete } from 'lucide-vue-next'

/**
 * Six-digit PIN pad for the door login: big keys (≥ 56 px), digits masked.
 * It takes focus when it appears, so a physical keyboard works at once:
 * digits, Backspace, Escape (clear) and Enter (send six digits again).
 * Emits `submit` once six digits are in. `disabled` (a locked PIN) greys
 * every key.
 */
const props = defineProps<{ busy?: boolean, disabled?: boolean, error?: string, label?: string }>()
const emit = defineEmits<{ submit: [pin: string] }>()
const pin = ref('')
const uid = useId()
const root = ref<HTMLElement | null>(null)
const off = computed(() => props.busy || props.disabled)

function press(d: string) {
  if (off.value || pin.value.length >= 6) return
  pin.value += d
  if (pin.value.length === 6) emit('submit', pin.value)
}
const back = () => (pin.value = pin.value.slice(0, -1))
const clear = () => (pin.value = '')

function onKey(e: KeyboardEvent) {
  if (/^\d$/.test(e.key)) press(e.key)
  else if (e.key === 'Backspace') back()
  else if (e.key === 'Escape') clear()
  else if (e.key === 'Enter') {
    if (pin.value.length === 6 && !off.value) emit('submit', pin.value)
  } else return
  e.preventDefault()
}

onMounted(() => root.value?.focus())

// Each try ends with busy → false: clear the pad for the next one (a
// successful login unmounts it anyway), even when the error text repeats.
watch(() => props.busy, (b, was) => {
  if (was && !b) clear()
})
defineExpose({ clear })
</script>

<template>
  <div ref="root" class="pinpad" role="group" :aria-labelledby="`${uid}-lbl`" tabindex="0" @keydown="onKey">
    <p :id="`${uid}-lbl`" class="section-lbl" style="margin:0;text-align:center;">{{ label ?? 'DOOR PIN' }}</p>
    <div class="dots" role="status" :aria-label="`${pin.length} of 6 digits entered`">
      <span v-for="i in 6" :key="i" class="dot" :class="{ on: i <= pin.length }" aria-hidden="true" />
    </div>
    <p v-if="error" role="alert" class="err">{{ error }}</p>
    <div class="keys">
      <button v-for="d in ['1', '2', '3', '4', '5', '6', '7', '8', '9']" :key="d" type="button" class="key btn-hud btn-hud-ghost" :disabled="off" @click="press(d)">
        {{ d }}
      </button>
      <button type="button" class="key btn-hud btn-hud-ghost" :disabled="off || !pin" aria-label="Clear" @click="clear">CLR</button>
      <button type="button" class="key btn-hud btn-hud-ghost" :disabled="off" @click="press('0')">0</button>
      <button type="button" class="key btn-hud btn-hud-ghost" :disabled="off || !pin" aria-label="Delete last digit" @click="back">
        <Delete style="width:22px;height:22px;" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.pinpad {
  display: grid;
  gap: 14px;
  width: 100%;
  max-width: 340px;
  margin: 0 auto;
  outline: none;
}
.pinpad:focus-visible {
  outline: 1px solid var(--color-primary-dim);
  outline-offset: 6px;
}
.dots {
  display: flex;
  justify-content: center;
  gap: 12px;
}
.dot {
  width: 14px;
  height: 14px;
  border: 1px solid var(--color-primary-dim);
}
.dot.on {
  background: var(--color-primary);
  box-shadow: 0 0 12px rgba(150, 248, 255, .4);
}
.err {
  margin: 0;
  text-align: center;
  font-size: 14px;
  color: var(--color-error);
}
.keys {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}
.key {
  height: 64px;
  min-height: 64px;
  font-family: var(--font-command);
  font-size: 24px;
  color: var(--color-on-surface);
}
</style>
