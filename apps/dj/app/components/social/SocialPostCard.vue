<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ScheduledPost } from '~/types/social'
import { useSocialStore } from '~/stores/social'
import { apiAssetUrl } from '~/utils/apiAssetUrl'
import { toZonedInput } from '~/utils/socialTime'

const props = defineProps<{
  post: ScheduledPost
}>()

const store = useSocialStore()

const expanded = ref(false)

// Edit form state
const editCaption = ref(props.post.caption)
const editScheduledAt = ref(toZonedInput(props.post.scheduledAtUtc, props.post.timezoneName))
const editTimezone = ref(props.post.timezoneName)
const saving = ref(false)

/** The editor always starts from what is saved now, not from mount time. */
function seed() {
  editCaption.value = props.post.caption
  editScheduledAt.value = toZonedInput(props.post.scheduledAtUtc, props.post.timezoneName)
  editTimezone.value = props.post.timezoneName
}

const isScheduled = computed(() => props.post.status === 'scheduled')

function toggleExpand() {
  if (!isScheduled.value) return
  // A tap on the open card must not close it and drop what was typed.
  if (expanded.value) return
  seed()
  expanded.value = true
}

function cancelEdit() {
  expanded.value = false
}

async function saveEdit() {
  if (saving.value) return
  saving.value = true
  const saved = await store.editPost(props.post.id, {
    caption: editCaption.value,
    scheduledAt: editScheduledAt.value,
    timezoneName: editTimezone.value,
  })
  saving.value = false
  // A failed save keeps the editor open with the typed text; the store says why.
  if (saved) expanded.value = false
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
        v-if="post.imageStorageKey"
        :src="apiAssetUrl(`/api/v1/social/posts/${post.id}/image`)"
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
          :disabled="saving"
          rows="3"
          class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-2 py-1.5 font-data text-xs text-on-surface resize-none focus:outline-none focus:border-primary transition-colors"
          placeholder="Caption..."
        />
        <input
          v-model="editScheduledAt"
          :disabled="saving"
          type="datetime-local"
          :aria-label="`Publish time (${editTimezone})`"
          class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-2 py-1.5 font-data text-xs text-on-surface focus:outline-none focus:border-primary transition-colors"
        >
        <p class="font-terminal tracking-terminal text-[10px] uppercase text-tertiary">
          TIME IN {{ editTimezone }}
        </p>
        <div class="flex gap-2 justify-end">
          <button
            :disabled="saving"
            class="ghost-border px-3 py-1 font-terminal tracking-terminal text-[10px] uppercase text-on-surface-variant hover:text-primary hover:border-primary/40 transition-colors"
            @click.stop="cancelEdit"
          >
            CANCEL
          </button>
          <button
            class="gradient-cta px-3 py-1 font-terminal tracking-terminal text-[10px] uppercase text-on-primary"
            :disabled="saving"
            @click.stop="saveEdit"
          >
            {{ saving ? 'SAVING…' : 'SAVE' }}
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
