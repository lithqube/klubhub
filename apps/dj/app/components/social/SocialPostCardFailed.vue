<script setup lang="ts">
import { computed } from 'vue'
import type { ScheduledPost } from '~/types/social'
import { useSocialStore } from '~/stores/social'

const props = defineProps<{
  post: ScheduledPost
}>()

const store = useSocialStore()

const formattedDateTime = computed(() => {
  if (!props.post.scheduledAtUtc) return ''
  const d = new Date(props.post.scheduledAtUtc)
  return d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).toUpperCase()
})

const platformTag = computed(() =>
  props.post.postType === 'story' ? 'INSTAGRAM REELS' : 'INSTAGRAM'
)

const statusLabel = computed(() =>
  props.post.status === 'permanently_failed' ? 'FAILED (PERMANENT)' : 'FAILED'
)

async function handleRetry() {
  await store.retryPost(props.post.id)
}

async function handleDownload() {
  await store.downloadImage(props.post.id)
}
</script>

<template>
  <div
    data-testid="failed-card"
    class="glass-panel border border-error/30 flex flex-col overflow-hidden"
  >
    <!-- Attention banner -->
    <div class="px-3 py-2 bg-error-container/40 border-b border-error/30 flex items-center gap-2">
      <span class="font-terminal tracking-terminal text-xs text-error uppercase">
        ATTENTION REQUIRED: SYNC ERROR
      </span>
    </div>

    <!-- Image thumbnail -->
    <div class="h-36 bg-surface-variant flex items-center justify-center flex-shrink-0 overflow-hidden">
      <img
        v-if="post.imageMinioPath"
        :src="`/api/v1/social/posts/${post.id}/image`"
        class="w-full h-full object-cover"
        alt="Post image"
        @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')"
      >
      <span v-else class="font-terminal tracking-terminal text-xs text-tertiary uppercase">
        NO IMAGE
      </span>
    </div>

    <!-- Card body -->
    <div class="flex flex-col gap-2 p-3 flex-1">
      <!-- Status badge -->
      <div class="flex items-center gap-2">
        <span class="px-2 py-0.5 font-terminal tracking-terminal text-[10px] uppercase bg-error text-white">
          {{ statusLabel }}
        </span>
      </div>

      <!-- Datetime -->
      <p class="font-terminal tracking-terminal text-xs text-on-surface">
        {{ formattedDateTime }}
      </p>

      <!-- Error reason -->
      <p v-if="post.lastError" class="font-data text-xs text-error/80">
        {{ post.lastError }}
      </p>

      <!-- Caption excerpt -->
      <p class="font-data text-xs text-on-surface-variant line-clamp-2">
        {{ post.caption }}
      </p>

      <!-- Platform tag -->
      <div class="mt-auto pt-1">
        <span class="ghost-border px-2 py-0.5 font-terminal tracking-terminal text-[10px] uppercase text-on-surface-variant">
          {{ platformTag }}
        </span>
      </div>
    </div>

    <!-- Footer actions -->
    <div class="flex items-center gap-3 px-3 py-2.5 border-t border-outline-variant/20">
      <button
        data-testid="retry-btn"
        class="font-terminal tracking-terminal text-[10px] uppercase text-error hover:text-error/80 transition-colors"
        @click="handleRetry"
      >
        RETRY SYNC
      </button>
      <button
        data-testid="download-btn"
        class="font-terminal tracking-terminal text-[10px] uppercase text-primary hover:text-primary/80 transition-colors"
        @click="handleDownload"
      >
        DOWNLOAD IMAGE
      </button>
    </div>
  </div>
</template>
