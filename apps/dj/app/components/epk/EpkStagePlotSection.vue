<script setup lang="ts">
import { ref, computed } from 'vue'
import { useEpkStore } from '~/stores/epk'
import { apiAssetUrl } from '~/utils/apiAssetUrl'

const store = useEpkStore()
const stagePlotPath = computed(() => store.stagePlotPath)

const fileInput = ref<HTMLInputElement>()
const uploadedFilename = ref<string>('')

const stagePlotUrl = computed(() => {
  if (!stagePlotPath.value) return ''
  return apiAssetUrl(`/api/v1/storage/proxy?path=${encodeURIComponent(stagePlotPath.value)}`)
})

function onDrop(e: DragEvent) {
  const files = e.dataTransfer?.files ?? null
  const file = files?.[0]
  if (file) handleFile(file)
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) handleFile(file)
}

async function handleFile(file: File) {
  uploadedFilename.value = file.name
  await store.uploadStagePlot(file)
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">STAGE PLOT</p>

    <!-- Image preview when uploaded -->
    <template v-if="stagePlotPath">
      <img
        :src="stagePlotUrl"
        data-testid="stage-plot-preview"
        class="w-full object-cover aspect-video"
        alt="Stage plot"
      >
      <p class="text-xs text-tertiary font-data">{{ uploadedFilename || stagePlotPath }}</p>
    </template>

    <!-- Drop zone when no image -->
    <template v-else>
      <div
        data-testid="stage-plot-upload-zone"
        class="ghost-border flex flex-col items-center justify-center gap-3 p-8 cursor-pointer"
        @dragover.prevent
        @drop.prevent="onDrop"
        @click="fileInput?.click()"
      >
        <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">
          DROP STAGE PLOT IMAGE
        </p>
        <p class="text-xs text-tertiary font-terminal uppercase">
          JPEG OR PNG
        </p>
        <input
          ref="fileInput"
          type="file"
          accept="image/jpeg,image/png"
          class="hidden"
          @change="onFileChange"
        >
      </div>
    </template>
  </div>
</template>
