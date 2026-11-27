<script setup lang="ts">
import { computed } from 'vue'
import { CalendarDays, Disc } from 'lucide-vue-next'
import {
  RELEASE_STATUS_LABELS,
  isReleaseOverdue,
  type Release,
} from '~/types/release'

const props = defineProps<{ release: Release }>()

const overdue = computed(() => isReleaseOverdue(props.release))
const releaseDate = computed(() => {
  const date = new Date(props.release.release_date)
  return Number.isNaN(date.getTime())
    ? props.release.release_date
    : date.toISOString().slice(0, 10)
})
</script>

<template>
  <NuxtLink
    :to="`/releases/${release.id}`"
    class="flex items-center justify-between gap-3 px-4 py-3 border-b border-outline/40 hover:border-primary/60 transition-colors"
    data-testid="release-row"
  >
    <div class="min-w-0">
      <div class="flex items-center gap-2">
        <Disc class="w-3 h-3 text-primary shrink-0" />
        <span class="font-terminal text-sm text-foreground truncate">{{ release.title }}</span>
      </div>
      <div class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary truncate">
        {{ release.artists || 'no artists yet' }}
      </div>
    </div>

    <div class="flex items-center gap-3 shrink-0">
      <span
        v-if="overdue"
        class="px-2 py-0.5 text-[9px] uppercase tracking-terminal text-error border border-error/60"
      >
        overdue
      </span>
      <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">
        {{ RELEASE_STATUS_LABELS[release.status] }}
      </span>
      <span class="flex items-center gap-1 font-terminal text-[9px] uppercase tracking-terminal text-tertiary">
        <CalendarDays class="w-3 h-3" />
        {{ releaseDate }}
      </span>
    </div>
  </NuxtLink>
</template>
