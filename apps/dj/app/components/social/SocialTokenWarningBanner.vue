<script setup lang="ts">
import { computed } from 'vue'
import type { SocialAccount } from '~/types/social'

const props = defineProps<{
  account: SocialAccount
}>()

const daysUntilExpiry = computed(() => {
  if (!props.account.tokenExpiry) return null
  const diff = new Date(props.account.tokenExpiry).getTime() - Date.now()
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
})

const showExpiryWarning = computed(() =>
  props.account.status === 'connected' &&
  daysUntilExpiry.value !== null &&
  daysUntilExpiry.value <= 7 &&
  daysUntilExpiry.value > 0
)

const showDisconnected = computed(() => props.account.status === 'disconnected')

async function reAuthorize() {
  const result = await $fetch<{ url: string }>('/api/v1/social/auth/url')
  window.location.href = result.url
}
</script>

<template>
  <!-- Disconnected: magenta banner -->
  <div
    v-if="showDisconnected"
    class="banner bg-secondary/10 border border-secondary px-6 py-4 flex items-center justify-between gap-6"
  >
    <span class="font-terminal tracking-terminal text-xs uppercase text-secondary">
      ● INSTAGRAM DISCONNECTED — RE-AUTHORIZE
    </span>
    <button
      class="ghost-border px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-secondary border-secondary hover:bg-secondary/10 transition-colors"
      @click="reAuthorize"
    >
      RE-AUTHORIZE
    </button>
  </div>

  <!-- Expiring soon: amber/orange banner -->
  <div
    v-else-if="showExpiryWarning"
    class="banner bg-amber-500/10 border border-amber-500 px-6 py-4 flex items-center justify-between gap-6"
  >
    <span class="font-terminal tracking-terminal text-xs uppercase text-amber-400">
      ⚠ INSTAGRAM TOKEN EXPIRING IN {{ daysUntilExpiry }} DAYS — RE-AUTHORIZE
    </span>
    <button
      class="ghost-border px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-amber-400 border-amber-400 hover:bg-amber-500/10 transition-colors"
      @click="reAuthorize"
    >
      RE-AUTHORIZE
    </button>
  </div>
</template>
