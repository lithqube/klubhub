<script setup lang="ts">
// RiderAttachmentList — list view used on /rider's ATTACHMENTS tab.
// Groups gigs by upcoming/past and renders one RiderAttachmentCard per
// gig (reusing the same component as in GigFormDialog).

import { computed, onMounted } from 'vue'
import { useGigStore } from '~/stores/gig'
import { useRiderStore } from '~/stores/rider'
import type { Gig } from '~/types/gig'
import RiderAttachmentCard from './RiderAttachmentCard.vue'
import { Loader, AlertCircle } from 'lucide-vue-next'

const gigStore = useGigStore()
const riderStore = useRiderStore()

const filter = ref<'all' | 'upcoming' | 'past'>('all')
const loading = ref(false)
const error = ref<string | null>(null)

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
  loading.value = true
  error.value = null
  try {
    if (gigStore.gigs.length === 0) {
      await gigStore.fetchGigs()
    }
    // Independent loads: one failing request must not stop the others.
    await Promise.allSettled([
      riderStore.loadTemplates(),
      ...gigStore.gigs.map(g => riderStore.loadAttachmentByGig(g.id)),
    ])
  } catch (e) {
    error.value = 'Could not load rider attachments: ' + (e instanceof Error ? e.message : String(e))
  } finally {
    loading.value = false
  }
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

    <div v-if="loading" class="glass" role="status" style="padding:16px;text-align:center;display:flex;align-items:center;justify-content:center;gap:8px;">
      <Loader class="animate-spin" style="width:13px;height:13px;color:var(--color-primary);" aria-hidden="true" />
      <span class="section-lbl">LOADING ATTACHMENTS...</span>
    </div>

    <div v-else-if="error" role="alert" class="glass" style="padding:20px;background:color-mix(in srgb, var(--color-error) 5%, var(--color-surface-container));border:1px solid color-mix(in srgb, var(--color-error) 15%, transparent);display:flex;align-items:center;gap:10px;flex-wrap:wrap;">
      <AlertCircle style="width:14px;height:14px;color:var(--color-error);" aria-hidden="true" />
      <span style="font-family:var(--font-data);font-size:12px;color:var(--color-error);flex:1;">{{ error }}</span>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" @click="loadAttachment">RETRY</button>
    </div>

    <div v-else-if="upcoming.length">
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

    <div v-else-if="past.length">
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

    <p v-if="!loading && !error && filtered.length === 0" class="text-tertiary font-terminal uppercase text-xs">
      No gigs match this filter.
    </p>
  </div>
</template>