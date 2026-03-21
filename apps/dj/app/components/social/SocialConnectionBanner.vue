<script setup lang="ts">
import { ref } from 'vue'
import { Camera } from 'lucide-vue-next'
import { useSocialStore } from '~/stores/social'

const store = useSocialStore()
const loading = ref(false)

async function connectInstagram() {
  loading.value = true
  try {
    const result = await $fetch<{ url: string }>('/api/v1/social/auth/url')
    window.location.href = result.url
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div
    v-if="!store.account || store.account.status === 'disconnected'"
    class="glass-panel border border-surface-variant px-6 py-5 flex items-center justify-between gap-6"
  >
    <!-- Left: icon + text -->
    <div class="flex items-center gap-4">
      <Camera class="w-6 h-6 text-on-surface-dim flex-shrink-0" aria-hidden="true" />
      <div>
        <p class="font-terminal tracking-terminal text-xs uppercase text-on-surface font-bold">
          CONNECT YOUR INSTAGRAM ACCOUNT
        </p>
        <p class="font-terminal tracking-terminal text-xs text-on-surface-dim mt-0.5">
          Schedule posts directly from your tracklist exports
        </p>
      </div>
    </div>

    <!-- Right: CTA button -->
    <button
      class="gradient-cta px-5 py-2.5 font-terminal tracking-terminal text-xs uppercase whitespace-nowrap flex items-center gap-2 disabled:opacity-60"
      :disabled="loading"
      @click="connectInstagram"
    >
      <span v-if="loading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" aria-hidden="true" />
      CONNECT INSTAGRAM
    </button>
  </div>
</template>
