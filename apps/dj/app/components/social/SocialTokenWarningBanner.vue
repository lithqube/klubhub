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
    class="glass-panel border border-error px-6 py-4 flex items-center justify-between gap-6"
  >
    <span class="font-terminal tracking-terminal text-xs uppercase text-error">
      INSTAGRAM DISCONNECTED — RE-AUTHORIZE
    </span>
    <button
      class="ghost-border px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-error border-error hover:bg-error-container/40 transition-colors"
      @click="reAuthorize"
    >
      RE-AUTHORIZE
    </button>
  </div>

  <!-- Expiring soon: amber warning banner -->
  <div
    v-else-if="showExpiryWarning"
    class="glass-panel border border-status-archived px-6 py-4 flex items-center justify-between gap-6"
  >
    <span class="font-terminal tracking-terminal text-xs uppercase text-status-archived">
      INSTAGRAM TOKEN EXPIRING IN {{ daysUntilExpiry }} DAYS — RE-AUTHORIZE
    </span>
    <button
      class="ghost-border px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-status-archived border-status-archived hover:bg-status-archived/10 transition-colors"
      @click="reAuthorize"
    >
      RE-AUTHORIZE
    </button>
  </div>
</template>
