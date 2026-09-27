<script setup lang="ts">
import { Undo2 } from 'lucide-vue-next'
import type { DoorToast } from '~/types/door'

/**
 * "Checked in — UNDO" for ~6 s after a door action. It sits at the top,
 * under the sticky header, away from the thumb zone, and the page hides it
 * as soon as a card opens, so UNDO is never under the next guest's ADMIT.
 * Older actions stay undoable from the RECENT list.
 */
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
  <div v-if="toast" class="toast" :class="toast.tone === 'warn' ? 'accent-bar-archived' : 'accent-bar-ready'" data-testid="door-toast">
    <span class="txt">{{ toast.text }}</span>
    <button v-if="toast.nonce" type="button" class="btn-hud btn-hud-ghost undo" @click="emit('undo', toast.nonce)">
      <Undo2 style="width:18px;height:18px;" aria-hidden="true" /> UNDO
    </button>
  </div>
</template>

<style scoped>
.toast {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 56px;
  padding: 0 0 0 12px;
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
