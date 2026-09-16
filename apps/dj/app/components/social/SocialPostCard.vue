<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ScheduledPost } from '~/types/social'
import { useSocialStore } from '~/stores/social'

const props = defineProps<{
  post: ScheduledPost
}>()

const store = useSocialStore()

const expanded = ref(false)

// Edit form state
const editCaption = ref(props.post.caption)
const editScheduledAt = ref(props.post.scheduledAtUtc.slice(0, 16)) // datetime-local format
const editTimezone = ref(props.post.timezoneName)

const isScheduled = computed(() => props.post.status === 'scheduled')

function toggleExpand() {
  if (!isScheduled.value) return
  expanded.value = !expanded.value
}

function cancelEdit() {
  editCaption.value = props.post.caption
  editScheduledAt.value = props.post.scheduledAtUtc.slice(0, 16)
  editTimezone.value = props.post.timezoneName
  expanded.value = false
}

async function saveEdit() {
  await store.editPost(props.post.id, {
    caption: editCaption.value,
    scheduledAt: editScheduledAt.value,
    timezoneName: editTimezone.value,
  })
  expanded.value = false
}

const statusLabel = computed(() => {
  switch (props.post.status) {
    case 'scheduled': return 'READY'
    case 'publishing': return 'PUBLISHING'
    case 'published': return 'PUBLISHED'
    case 'draft': return 'DRAFT'
    default: return props.post.status.toUpperCase()
  }
})

const statusClass = computed(() => {
  switch (props.post.status) {
    case 'scheduled': return 'ghost-border text-primary'
    case 'publishing': return 'bg-primary text-on-primary animate-pulse'
    case 'published': return 'bg-surface-container-high text-tertiary'
    case 'draft': return 'ghost-border text-secondary'
    default: return 'bg-surface-container-high text-tertiary'
  }
})

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
</script>

<template>
  <div
    data-testid="post-card"
    class="glass-panel border border-outline-variant/20 flex flex-col overflow-hidden"
    :class="isScheduled ? 'cursor-pointer' : 'cursor-default'"
    @click="toggleExpand"
  >
    <!-- Image thumbnail -->
    <div class="h-40 bg-surface-variant flex items-center justify-center flex-shrink-0 overflow-hidden">
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
        <span
          class="px-2 py-0.5 font-terminal tracking-terminal text-[10px] uppercase"
          :class="statusClass"
        >
          {{ statusLabel }}
        </span>
      </div>

      <!-- Datetime -->
      <p class="font-terminal tracking-terminal text-xs text-on-surface">
        {{ formattedDateTime }}
      </p>

      <!-- Caption excerpt (collapsed view) -->
      <div v-if="!expanded">
        <p class="font-data text-xs text-on-surface-variant line-clamp-2">
          {{ post.caption }}
        </p>
      </div>

      <!-- Inline edit form (expanded view) -->
      <div v-if="expanded" class="space-y-2" @click.stop>
        <textarea
          v-model="editCaption"
          rows="3"
          class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-2 py-1.5 font-data text-xs text-on-surface resize-none focus:outline-none focus:border-primary transition-colors"
          placeholder="Caption..."
        />
        <input
          v-model="editScheduledAt"
          type="datetime-local"
          class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-2 py-1.5 font-data text-xs text-on-surface focus:outline-none focus:border-primary transition-colors"
        >
        <div class="flex gap-2 justify-end">
          <button
            class="ghost-border px-3 py-1 font-terminal tracking-terminal text-[10px] uppercase text-on-surface-variant hover:text-primary hover:border-primary/40 transition-colors"
            @click.stop="cancelEdit"
          >
            CANCEL
          </button>
          <button
            class="gradient-cta px-3 py-1 font-terminal tracking-terminal text-[10px] uppercase text-on-primary"
            @click.stop="saveEdit"
          >
            SAVE
          </button>
        </div>
      </div>

      <!-- Platform tag -->
      <div class="mt-auto pt-2">
        <span class="ghost-border px-2 py-0.5 font-terminal tracking-terminal text-[10px] uppercase text-on-surface-variant">
          {{ platformTag }}
        </span>
      </div>
    </div>
  </div>
</template>
