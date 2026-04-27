<script setup lang="ts">
import type { SocialAccount } from '~/types/social'

const props = defineProps<{
  account: SocialAccount | null
}>()

const emit = defineEmits<{
  disconnect: []
  newPost: []
}>()
</script>

<template>
  <div class="page-header">
    <div>
      <div class="page-title">SOCIAL</div>
      <div class="page-sub">INSTAGRAM SCHEDULER</div>
    </div>
    <div style="display:flex;align-items:center;gap:8px;">
      <!-- Connected account indicator -->
      <template v-if="props.account && props.account.status === 'connected'">
        <div style="display:flex;align-items:center;gap:6px;" class="hidden sm:flex">
          <div class="pulse-dot" aria-hidden="true" />
          <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);">
            @{{ props.account.accountName }}
          </span>
        </div>
        <button
          class="btn-hud btn-hud-ghost btn-hud-sm"
          style="padding:0 12px;"
          @click="emit('disconnect')"
        >
          DISCONNECT
        </button>
      </template>
      <button
        class="btn-hud btn-hud-cta"
        style="padding:0 14px;"
        @click="emit('newPost')"
      >
        + NEW POST
      </button>
    </div>
  </div>
</template>
