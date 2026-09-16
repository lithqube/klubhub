<script setup lang="ts">
import { watch, computed } from 'vue'
import { useSocialPostForm } from '~/composables/useSocialPostForm'
import { useSocialStore } from '~/stores/social'
import { useTracklistStore } from '~/stores/tracklist'

const props = defineProps<{
  prefilledImageId: string | null
}>()

const store = useSocialStore()
const tracklistStore = useTracklistStore()

const {
  postType,
  caption,
  scheduledAt,
  timezoneName,
  timezoneSearch,
  imageId,
  imageFile,
  charLimit,
  charCount,
  isOverLimit,
  timezoneOptions,
  scheduledAtUTCPreview,
  generateCaption,
  resetForm,
} = useSocialPostForm()

// Watch for prefilled image id: when it changes to non-null, auto-gen caption
watch(
  () => props.prefilledImageId,
  (newVal) => {
    if (newVal) {
      imageId.value = newVal
      const tl = tracklistStore.tracklist
      if (tl) {
        const avgBpm = tracklistStore.tracks.length > 0
          ? tracklistStore.tracks.reduce((sum: number, t: any) => sum + (t.bpm ?? 0), 0) / tracklistStore.tracks.length
          : 0
        generateCaption({
          title: (tl as any).title ?? '',
          trackCount: tracklistStore.tracks.length,
          avgBpm,
        })
      }
    }
  }
)

const isSubmitDisabled = computed(() => {
  const hasImage = !!(imageId.value || imageFile.value)
  return !caption.value || !scheduledAt.value || !hasImage || isOverLimit.value
})

async function handleSubmit() {
  if (isSubmitDisabled.value) return
  await store.createPost({
    postType: postType.value,
    caption: caption.value,
    scheduledAt: scheduledAt.value,
    timezoneName: timezoneName.value,
    imageId: imageId.value ?? undefined,
    imageFile: imageFile.value ?? undefined,
  })
  store.closeComposePanel()
  resetForm()
  await store.loadPosts()
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.files && input.files[0]) {
    imageFile.value = input.files[0]
  }
}

function removeImage() {
  imageId.value = null
  imageFile.value = null
}
</script>

<template>
  <div data-compose-panel class="glass-panel border border-outline-variant/20 p-6 space-y-5">

    <!-- Header -->
    <div class="flex items-center justify-between">
      <h3 class="font-command font-bold text-primary text-sm uppercase tracking-command">
        NEW POST
      </h3>
      <button
        class="font-terminal tracking-terminal text-xs text-tertiary uppercase hover:text-on-surface transition-colors"
        @click="store.closeComposePanel()"
      >
        CLOSE ×
      </button>
    </div>

    <!-- Feed / Story toggle -->
    <div class="flex items-center gap-3">
      <span
        class="font-terminal tracking-terminal text-xs uppercase"
        :class="postType === 'feed' ? 'text-primary' : 'text-tertiary'"
      >FEED</span>
      <button
        class="relative w-8 h-[18px] rounded-none border transition-colors flex-shrink-0"
        :class="postType === 'story' ? 'bg-primary/20 border-primary' : 'bg-surface-container-high border-primary/20'"
        role="switch"
        :aria-checked="postType === 'story'"
        @click="postType = postType === 'feed' ? 'story' : 'feed'"
      >
        <span
          class="absolute top-0.5 w-3 h-3 transition-transform"
          :class="postType === 'story' ? 'translate-x-4 bg-primary' : 'translate-x-0.5 bg-tertiary'"
        />
      </button>
      <span
        class="font-terminal tracking-terminal text-xs uppercase"
        :class="postType === 'story' ? 'text-primary' : 'text-tertiary'"
      >STORY</span>
    </div>

    <!-- Caption textarea -->
    <div class="space-y-1.5">
      <label class="font-terminal tracking-terminal text-xs text-on-surface-variant uppercase">
        CAPTION
      </label>
      <textarea
        v-model="caption"
        rows="4"
        class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-3 py-2 font-data text-sm text-on-surface resize-none focus:outline-none focus:border-primary transition-colors"
        :class="isOverLimit ? 'ring-1 ring-error text-error' : ''"
        placeholder="Write your caption..."
      />
      <!-- Character counter for feed; note for story -->
      <div v-if="charLimit !== null" class="text-right">
        <span
          class="font-data text-xs"
          :class="isOverLimit ? 'text-error' : 'text-tertiary'"
        >{{ charCount }} / {{ charLimit }}</span>
      </div>
      <div v-else class="text-right">
        <span class="font-data text-xs text-tertiary italic">
          Captions are not shown on Stories
        </span>
      </div>
    </div>

    <!-- Image attachment -->
    <div class="space-y-1.5">
      <label class="font-terminal tracking-terminal text-xs text-on-surface-variant uppercase">
        IMAGE
      </label>

      <!-- Prefilled image from tracklist -->
      <div
        v-if="imageId"
        class="flex items-center gap-3 px-3 py-2 border border-dashed border-primary/30 bg-primary/5"
      >
        <span class="font-terminal tracking-terminal text-xs text-primary uppercase flex-1">
          ⚡ ATTACHED FROM TRACKLIST
        </span>
        <button
          class="font-terminal tracking-terminal text-xs text-tertiary hover:text-on-surface transition-colors"
          @click="removeImage()"
        >
          ×
        </button>
      </div>

      <!-- File uploaded -->
      <div
        v-else-if="imageFile"
        class="flex items-center gap-3 px-3 py-2 border border-dashed border-outline-variant/40 bg-surface-container-high"
      >
        <span class="font-data text-xs text-on-surface flex-1 truncate">{{ imageFile.name }}</span>
        <button
          class="font-terminal tracking-terminal text-xs text-tertiary hover:text-on-surface transition-colors"
          @click="removeImage()"
        >
          ×
        </button>
      </div>

      <!-- Upload input -->
      <div v-else>
        <label class="block cursor-pointer">
          <input
            type="file"
            accept="image/*"
            class="sr-only"
            @change="handleFileChange"
          >
          <span class="ghost-border px-4 py-2 font-terminal tracking-terminal text-xs uppercase text-on-surface-variant hover:text-primary hover:border-primary/40 transition-colors inline-block">
            UPLOAD IMAGE
          </span>
        </label>
      </div>
    </div>

    <!-- Datetime + timezone (side by side) -->
    <div class="grid grid-cols-2 gap-3">
      <div class="space-y-1.5">
        <label class="font-terminal tracking-terminal text-xs text-on-surface-variant uppercase">
          SCHEDULE DATE &amp; TIME
        </label>
        <input
          v-model="scheduledAt"
          type="datetime-local"
          class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-3 py-2 font-data text-sm text-on-surface focus:outline-none focus:border-primary transition-colors"
        >
      </div>

      <div class="space-y-1.5">
        <label class="font-terminal tracking-terminal text-xs text-on-surface-variant uppercase">
          TIMEZONE
        </label>
        <!-- Simple select for timezone -->
        <div class="relative">
          <input
            v-model="timezoneSearch"
            type="text"
            :placeholder="timezoneName"
            class="w-full bg-surface-container-high border-0 border-l-2 border-transparent px-3 py-2 font-data text-xs text-on-surface focus:outline-none focus:border-primary transition-colors"
          >
          <div
            v-if="timezoneSearch"
            class="glass absolute top-full left-0 right-0 z-20 max-h-40 overflow-y-auto"
          >
            <button
              v-for="tz in timezoneOptions.slice(0, 20)"
              :key="tz"
              class="block w-full text-left px-3 py-1.5 font-data text-xs text-on-surface hover:bg-primary/[0.08] transition-colors"
              @click="() => { timezoneName = tz; timezoneSearch = '' }"
            >
              {{ tz }}
            </button>
          </div>
        </div>
        <p v-if="scheduledAtUTCPreview" class="font-data text-xs text-tertiary">
          {{ scheduledAtUTCPreview }}
        </p>
      </div>
    </div>

    <!-- Submit -->
    <div class="flex justify-end">
      <button
        data-testid="submit-btn"
        class="gradient-cta px-6 py-2.5 font-terminal tracking-terminal text-xs uppercase text-on-primary disabled:opacity-40 disabled:cursor-not-allowed"
        :disabled="isSubmitDisabled || undefined"
        @click="handleSubmit"
      >
        + SCHEDULE POST
      </button>
    </div>

  </div>
</template>
