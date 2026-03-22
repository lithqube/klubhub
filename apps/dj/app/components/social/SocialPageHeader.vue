<script setup lang="ts">
import type { SocialAccount } from '~/types/social'

const props = defineProps<{
  account: SocialAccount | null
}>()

const emit = defineEmits<{
  disconnect: []
}>()
</script>

<template>
  <header class="px-4 md:px-8 py-4 md:py-5 border-b border-outline-variant/20 flex items-center justify-between gap-3 flex-shrink-0">
    <!-- Left: title + breadcrumb -->
    <div>
      <h1 class="font-command font-bold italic text-primary text-xl uppercase tracking-wide">
        SOCIAL SCHEDULER
      </h1>
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase mt-0.5">
        STUDIO / SOCIAL
      </p>
    </div>

    <!-- Right: account status — Fitts's Law: disconnect button min-h-[44px] -->
    <div v-if="props.account && props.account.status === 'connected'" class="flex items-center gap-2 md:gap-3 flex-shrink-0">
      <span class="w-2 h-2 rounded-full bg-emerald-400 flex-shrink-0" aria-hidden="true" />
      <span class="font-terminal tracking-terminal text-xs text-on-surface uppercase hidden sm:inline">
        @{{ props.account.accountName }}
      </span>
      <button
        class="ghost-border px-3 min-h-[44px] md:min-h-0 md:py-1.5 font-terminal tracking-terminal text-xs uppercase text-on-surface-variant hover:text-primary hover:border-primary/40 transition-colors"
        @click="emit('disconnect')"
      >
        DISCONNECT
      </button>
    </div>
  </header>
</template>
