<script setup lang="ts">
import { ref, computed } from 'vue'
import { useEpkStore } from '~/stores/epk'

const store = useEpkStore()

const photoPaths = computed(() => store.photoPaths)
const canAddPhoto = computed(() => store.canAddPhoto)
const photoCount = computed(() => store.photoCount)

const fileInput = ref<HTMLInputElement>()

function getPhotoUrl(path: string): string {
  return `/api/v1/storage/proxy?path=${encodeURIComponent(path)}`
}

async function onFilesSelected(files: FileList | null) {
  if (!files) return
  for (const file of Array.from(files)) {
    await store.uploadPhoto(file)
  }
}

function onDrop(e: DragEvent) {
  onFilesSelected(e.dataTransfer?.files ?? null)
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  onFilesSelected(input.files ?? null)
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">PHOTOS</p>

    <!-- Upload zone or limit label -->
    <div
      v-if="canAddPhoto"
      data-testid="photo-upload-zone"
      class="ghost-border flex flex-col items-center justify-center gap-2 p-6 cursor-pointer"
      @dragover.prevent
      @drop.prevent="onDrop"
      @click="fileInput?.click()"
    >
      <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">
        DROP PHOTOS HERE
      </p>
      <p class="text-xs text-tertiary font-terminal uppercase">
        {{ photoCount }} / 20 UPLOADED — JPEG OR PNG
      </p>
      <input
        ref="fileInput"
        type="file"
        accept="image/jpeg,image/png"
        multiple
        class="hidden"
        @change="onFileChange"
      />
    </div>

    <p
      v-else
      data-testid="photo-limit-label"
      class="text-xs tracking-terminal text-tertiary uppercase font-terminal text-center py-3"
    >
      20 / 20 PHOTOS — REMOVE ONE TO ADD MORE
    </p>

    <!-- Photo grid -->
    <div
      v-if="photoPaths.length > 0"
      data-testid="photo-grid"
      class="photo-grid"
    >
      <div
        v-for="(path, index) in photoPaths"
        :key="path"
        data-testid="photo-item"
        class="relative aspect-square"
      >
        <img
          :src="getPhotoUrl(path)"
          class="w-full h-full object-cover"
          alt="EPK photo"
        />
        <!-- Delete overlay on hover -->
        <button
          :data-testid="`photo-delete-${index}`"
          class="absolute top-1 right-1 w-6 h-6 bg-surface/80 text-on-surface flex items-center justify-center text-xs opacity-0 hover:opacity-100 transition-opacity cursor-pointer"
          @click="store.deletePhoto(path)"
        >
          &times;
        </button>
      </div>
    </div>
  </div>
</template>
