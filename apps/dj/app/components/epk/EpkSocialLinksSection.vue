<script setup lang="ts">
import { ref, onMounted } from 'vue'
import Input from '~/components/ui/input/Input.vue'

type SocialLinks = Record<string, string>

const PLATFORMS = [
  { key: 'instagram', label: 'INSTAGRAM' },
  { key: 'soundcloud', label: 'SOUNDCLOUD' },
  { key: 'spotify', label: 'SPOTIFY' },
  { key: 'mixcloud', label: 'MIXCLOUD' },
  { key: 'residentAdvisor', label: 'RESIDENT ADVISOR' },
  { key: 'bandcamp', label: 'BANDCAMP' },
  { key: 'youtube', label: 'YOUTUBE' },
] as const

const links = ref<SocialLinks>({
  instagram: '',
  soundcloud: '',
  spotify: '',
  mixcloud: '',
  residentAdvisor: '',
  bandcamp: '',
  youtube: '',
})

// Load existing social links from settings API
onMounted(async () => {
  try {
    const data = await $fetch<Record<string, any>>('/api/v1/settings')
    if (data.social_links && typeof data.social_links === 'object') {
      links.value = {
        instagram: data.social_links.instagram ?? '',
        soundcloud: data.social_links.soundcloud ?? '',
        spotify: data.social_links.spotify ?? '',
        mixcloud: data.social_links.mixcloud ?? '',
        residentAdvisor: data.social_links.residentAdvisor ?? data.social_links.resident_advisor ?? '',
        bandcamp: data.social_links.bandcamp ?? '',
        youtube: data.social_links.youtube ?? '',
      }
    }
  } catch {
    // Ignore load errors — blank fields is acceptable default
  }
})

let saveTimer: ReturnType<typeof setTimeout> | null = null

function scheduleSave() {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    try {
      await $fetch('/api/v1/settings', {
        method: 'put',
        body: { social_links: links.value },
      })
    } catch {
      // Ignore save errors silently
    }
  }, 1500)
}

function onLinkChange(key: string, value: string | number) {
  links.value = { ...links.value, [key]: String(value) }
  scheduleSave()
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">SOCIAL LINKS</p>

    <div class="space-y-3">
      <div
        v-for="platform in PLATFORMS"
        :key="platform.key"
        class="space-y-1"
      >
        <label class="text-xs tracking-terminal text-tertiary uppercase font-terminal">
          {{ platform.label }}
        </label>
        <Input
          :model-value="links[platform.key]"
          :data-testid="`social-input-${platform.key}`"
          :placeholder="`${platform.label} URL`"
          @update:model-value="onLinkChange(platform.key, $event)"
        />
      </div>
    </div>
  </div>
</template>
