<script setup lang="ts">
import { Undo2 } from 'lucide-vue-next'
import type { DoorToast } from '~/types/door'

/** "Checked in — UNDO" for ~6 s after every door action. */
const props = defineProps<{ toast: DoorToast | null }>()
const emit = defineEmits<{ undo: [nonce: string], expire: [] }>()
const UNDO_MS = 6000
let timer: ReturnType<typeof setTimeout> | undefined

watch(() => props.toast?.id, () => {
  clearTimeout(timer)
  if (props.toast) timer = setTimeout(() => emit('expire'), UNDO_MS)
})
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div v-if="toast" class="toast glass" :class="toast.tone === 'warn' ? 'accent-bar-archived' : 'accent-bar-ready'" data-testid="door-toast">
    <span class="txt">{{ toast.text }}</span>
    <button v-if="toast.nonce" type="button" class="btn-hud btn-hud-ghost undo" @click="emit('undo', toast.nonce)">
      <Undo2 style="width:18px;height:18px;" aria-hidden="true" /> UNDO
    </button>
  </div>
</template>

<style scoped>
.toast {
  position: fixed;
  left: 50%;
  bottom: calc(16px + env(safe-area-inset-bottom));
  transform: translateX(-50%);
  z-index: 40;
  width: calc(100% - 32px);
  max-width: 520px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 6px 6px 14px;
  background: var(--color-surface-container-high);
}
.txt {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-on-surface);
  overflow-wrap: anywhere;
}
.undo {
  min-height: 56px;
  height: 56px;
  font-size: 12px;
  flex-shrink: 0;
}
</style>
