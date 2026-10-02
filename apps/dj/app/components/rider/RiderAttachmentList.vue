<script setup lang="ts">
// RiderAttachmentList — list view used on /rider's ATTACHMENTS tab.
// Groups gigs by upcoming/past and renders one RiderAttachmentCard per
// gig (reusing the same component as in GigFormDialog).

import { computed, onMounted } from 'vue'
import { useGigStore } from '~/stores/gig'
import { useRiderStore } from '~/stores/rider'
import type { Gig } from '~/types/gig'
import RiderAttachmentCard from './RiderAttachmentCard.vue'

const gigStore = useGigStore()
const riderStore = useRiderStore()

const filter = ref<'all' | 'upcoming' | 'past'>('all')

const filtered = computed(() => {
  const now = Date.now()
  return gigStore.gigs.filter((g: Gig) => {
    const t = new Date(g.date).getTime()
    if (filter.value === 'upcoming') return t >= now
    if (filter.value === 'past') return t < now
    return true
  })
})

const upcoming = computed(() => filtered.value.filter(g => new Date(g.date).getTime() >= Date.now()))
const past = computed(() => filtered.value.filter(g => new Date(g.date).getTime() < Date.now()))

onMounted(async () => {
  if (gigStore.gigs.length === 0) {
    await gigStore.fetchGigs()
  }
  // Independent loads: one failing request must not stop the others.
  await Promise.allSettled([
    riderStore.loadTemplates(),
    ...gigStore.gigs.map(g => riderStore.loadAttachmentByGig(g.id)),
  ])
})

async function onOpenEditor(gigId: string): Promise<void> {
  await navigateTo({ path: '/rider', query: { gig: gigId } })
}
</script>

<template>
  <div class="space-y-4" data-testid="rider-attachment-list">
    <div class="flex gap-2">
      <button
        v-for="f in (['all', 'upcoming', 'past'] as const)"
        :key="f"
        type="button"
        class="btn-hud btn-hud-ghost"
        :data-testid="`rider-filter-${f}`"
        @click="filter = f"
      >{{ f.toUpperCase() }}</button>
    </div>

    <div v-if="upcoming.length">
      <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal mb-2">UPCOMING</p>
      <div class="space-y-2">
        <RiderAttachmentCard
          v-for="g in upcoming"
          :key="g.id"
          :gig="g"
          @open-editor="onOpenEditor"
        />
      </div>
    </div>

    <div v-if="past.length">
      <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal mb-2 mt-4">PAST</p>
      <div class="space-y-2">
        <RiderAttachmentCard
          v-for="g in past"
          :key="g.id"
          :gig="g"
          @open-editor="onOpenEditor"
        />
      </div>
    </div>

    <p v-if="filtered.length === 0" class="text-tertiary font-terminal uppercase text-xs">
      No gigs match this filter.
    </p>
  </div>
</template>