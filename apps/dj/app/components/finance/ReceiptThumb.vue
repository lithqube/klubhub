<script setup lang="ts">
// Square preview of one receipt: the image itself, or a PDF/file glyph when
// there is nothing to show (PDF, or an image still waiting without a preview).
import { ref, watch } from 'vue'
import { FileText } from 'lucide-vue-next'

const props = defineProps<{ src: string | null; image: boolean }>()
const broken = ref(false)
watch(() => props.src, () => { broken.value = false })
</script>

<template>
  <span class="rt" aria-hidden="true">
    <img v-if="image && src && !broken" class="rt-img" :src="src" alt="" loading="lazy" @error="broken = true">
    <FileText v-else class="rt-icon" :size="20" :stroke-width="1.5" />
  </span>
</template>

<style scoped>
.rt { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 44px; height: 44px; overflow: hidden; background: var(--color-surface-container-high); border: 1px dashed color-mix(in srgb, var(--color-on-surface) 22%, transparent); }
.rt-img { width: 100%; height: 100%; object-fit: cover; display: block; }
.rt-icon { color: var(--color-tertiary); }
</style>
