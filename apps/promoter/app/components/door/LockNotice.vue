<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { timeLabel } from '~/utils/datetime'

/**
 * "PIN LOCKED · TRY AGAIN AT 23:47" with a live countdown, after too many
 * wrong door PINs. Emits `over` when the lock has run out. The countdown is
 * hidden from screen readers (it would be read every second); the alert
 * text says the time once.
 */
const props = defineProps<{ until: number }>()
const emit = defineEmits<{ over: [] }>()
const now = useNow({ interval: 1000 })
const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
const at = computed(() => timeLabel(new Date(props.until).toISOString(), tz))
const left = computed(() => Math.max(0, Math.ceil((props.until - now.value.getTime()) / 1000)))
const clock = computed(() => `${Math.floor(left.value / 60)}:${String(left.value % 60).padStart(2, '0')}`)
watch(left, (n) => {
  if (n === 0) emit('over')
}, { immediate: true })
</script>

<template>
  <div class="lock glass accent-bar-archived" data-testid="door-pin-locked">
    <p role="alert" class="head">PIN LOCKED · TRY AGAIN AT {{ at }}</p>
    <p class="count" aria-hidden="true">{{ clock }}</p>
    <p class="txt">Too many wrong PINs. A manager can unlock it now by generating a new PIN in the event’s DOOR tab.</p>
  </div>
</template>

<style scoped>
.lock {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
}
.head {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 14px;
  font-weight: 700;
  letter-spacing: .05em;
  color: var(--color-status-archived);
}
.count {
  margin: 0;
  font-family: var(--font-command);
  font-size: 28px;
  font-weight: 700;
  color: var(--color-on-surface);
}
.txt {
  margin: 0;
  font-size: 14px;
  color: var(--color-on-surface-variant);
}
</style>
