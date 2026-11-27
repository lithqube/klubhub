<script setup lang="ts">
import { computed, ref } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import {
  RELEASE_PLATFORM_LABELS,
  type PlatformLink,
  type ReleasePlatform,
} from '~/types/release'

const props = defineProps<{
  links: PlatformLink[]
  busy?: boolean
}>()

const emit = defineEmits<{
  save: [platform: ReleasePlatform, url: string]
  remove: [platform: ReleasePlatform]
}>()

const platforms: ReleasePlatform[] = ['beatport', 'bandcamp', 'spotify', 'soundcloud']

const platform = ref<ReleasePlatform>('beatport')
const url = ref('')

const current = computed(() => props.links.find((l) => l.platform === platform.value))

function save(): void {
  if (!url.value.trim()) return
  emit('save', platform.value, url.value.trim())
  url.value = ''
}
</script>

<template>
  <section class="border border-outline/50 p-3" data-testid="release-links">
    <h3 class="font-terminal text-[10px] uppercase tracking-terminal text-tertiary mb-2">
      platform links
    </h3>

    <ul v-if="links.length" class="mb-3 space-y-1">
      <li
        v-for="link in links"
        :key="link.id"
        class="flex items-center justify-between gap-2"
      >
        <a
          :href="link.url"
          target="_blank"
          rel="noopener noreferrer"
          class="font-terminal text-[10px] uppercase tracking-terminal text-primary hover:underline"
        >
          {{ RELEASE_PLATFORM_LABELS[link.platform] }}
        </a>
        <button
          type="button"
          class="text-tertiary hover:text-error transition-colors"
          :disabled="busy"
          :aria-label="`Remove ${RELEASE_PLATFORM_LABELS[link.platform]} link`"
          @click="emit('remove', link.platform)"
        >
          <Trash2 class="w-3 h-3" />
        </button>
      </li>
    </ul>
    <p v-else class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary mb-2">
      No links yet.
    </p>

    <div class="flex gap-2 items-end">
      <label>
        <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">Platform</span>
        <select
          v-model="platform"
          class="mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
          data-testid="link-platform"
        >
          <option v-for="p in platforms" :key="p" :value="p">{{ RELEASE_PLATFORM_LABELS[p] }}</option>
        </select>
      </label>
      <label class="flex-1">
        <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">URL</span>
        <input
          v-model="url"
          type="url"
          placeholder="https://"
          class="w-full mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
          data-testid="link-url"
        >
      </label>
      <button
        type="button"
        class="px-2 py-1 border border-primary text-primary font-terminal text-[9px] uppercase tracking-terminal disabled:opacity-50"
        :disabled="!url.trim() || busy"
        data-testid="link-save"
        @click="save"
      >
        Save
      </button>
    </div>

    <p v-if="current" class="mt-2 font-terminal text-[9px] uppercase tracking-terminal text-tertiary">
      {{ RELEASE_PLATFORM_LABELS[current.platform] }} link saved — saving again replaces it.
    </p>
  </section>
</template>
