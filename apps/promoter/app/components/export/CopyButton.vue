<script setup lang="ts">
const props = defineProps<{ text: string, label?: string, caption?: string }>()
const state = ref<'idle' | 'copied' | 'blocked'>('idle')
let timer: ReturnType<typeof setTimeout> | undefined

async function copy() {
  try {
    await navigator.clipboard.writeText(props.text)
    state.value = 'copied'
  } catch {
    // Clipboard blocked (permissions, insecure context): the value stays visible to select by hand.
    state.value = 'blocked'
  }
  clearTimeout(timer)
  timer = setTimeout(() => (state.value = 'idle'), 2500)
}
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;min-width:72px;" :aria-label="`${label ?? 'Copy'}`" @click="copy">
    <span aria-live="polite">{{ state === 'copied' ? 'COPIED' : state === 'blocked' ? 'SELECT & ⌘C' : caption ?? 'COPY' }}</span>
  </button>
</template>
