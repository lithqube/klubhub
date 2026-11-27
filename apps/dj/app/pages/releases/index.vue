<script setup lang="ts">
// /releases — the release planner (Phase 6): every planned release, ordered
// by release date, with the overdue ones flagged. The timeline strip shows
// the release dates and their deadlines on one row.

import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Plus } from 'lucide-vue-next'
import { useReleaseStore } from '~/stores/releases'
import { isReleaseOverdue, type ReleaseInput } from '~/types/release'
import ReleaseListRow from '~/components/release/ReleaseListRow.vue'

useHead({ title: 'Releases: KlubHub DJ' })

const store = useReleaseStore()
const { releases, loading, error } = storeToRefs(store)

const formOpen = ref(false)
const title = ref('')
const artists = ref('')
const releaseDate = ref('')
const saving = ref(false)

const overdueCount = computed(() => releases.value.filter((r) => isReleaseOverdue(r)).length)

// The timeline strip: the release dates and every deadline, soonest first.
const timelineDates = computed(() => {
  const dates = releases.value.map((r) => r.release_date).filter(Boolean)
  return [...new Set(dates)].sort()
})

async function load(): Promise<void> {
  await store.fetchReleases()
}

onMounted(load)

function openForm(): void {
  title.value = ''
  artists.value = ''
  releaseDate.value = ''
  formOpen.value = true
}

async function create(): Promise<void> {
  if (!title.value.trim() || !releaseDate.value) return
  saving.value = true
  const payload: ReleaseInput = {
    title: title.value.trim(),
    artists: artists.value.trim(),
    release_date: releaseDate.value,
    format: 'digital',
  }
  const created = await store.createRelease(payload)
  saving.value = false
  if (created) formOpen.value = false
}
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <header class="px-4 pt-6 pb-4 border-b border-outline/40">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="font-terminal text-lg tracking-terminal uppercase">Releases</h1>
          <p class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">
            {{ releases.length }} planned · {{ overdueCount }} overdue
          </p>
        </div>
        <button
          type="button"
          class="flex items-center gap-1 px-3 py-2 border border-primary text-primary text-[9px] uppercase tracking-terminal hover:bg-primary hover:text-background transition-colors"
          data-testid="new-release"
          @click="openForm"
        >
          <Plus class="w-3 h-3" /> New
        </button>
      </div>
    </header>

    <p v-if="error" class="px-4 py-2 text-[10px] uppercase tracking-terminal text-error">
      {{ error }}
    </p>

    <section v-if="timelineDates.length" class="px-4 py-3 border-b border-outline/40">
      <p class="font-terminal text-[8px] uppercase tracking-terminal text-tertiary mb-2">
        timeline_view
      </p>
      <div class="flex gap-2 overflow-x-auto pb-1">
        <div
          v-for="date in timelineDates"
          :key="date"
          class="shrink-0 px-2 py-1 border border-outline/60 font-terminal text-[9px] uppercase tracking-terminal text-tertiary"
        >
          {{ date }}
        </div>
      </div>
    </section>

    <section v-if="loading" class="px-4 py-6 font-terminal text-[10px] uppercase tracking-terminal text-tertiary">
      Loading releases…
    </section>

    <section v-else-if="!releases.length" class="px-4 py-6 font-terminal text-[10px] uppercase tracking-terminal text-tertiary">
      No releases yet. Plan your first one.
    </section>

    <section v-else class="divide-y divide-outline/30">
      <ReleaseListRow v-for="release in releases" :key="release.id" :release="release" />
    </section>

    <section
      v-if="formOpen"
      class="fixed inset-0 z-40 flex items-end sm:items-center justify-center bg-black/60"
      @click.self="formOpen = false"
    >
      <div class="w-full sm:max-w-md bg-surface-container border border-outline/60 p-4">
        <h2 class="font-terminal text-sm tracking-terminal uppercase mb-3">New release</h2>
        <label class="block mb-2">
          <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">Title</span>
          <input
            v-model="title"
            type="text"
            class="w-full mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
            data-testid="release-title"
          >
        </label>
        <label class="block mb-2">
          <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">Artists</span>
          <input
            v-model="artists"
            type="text"
            class="w-full mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
            data-testid="release-artists"
          >
        </label>
        <label class="block mb-3">
          <span class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">Release date</span>
          <input
            v-model="releaseDate"
            type="date"
            class="w-full mt-1 px-2 py-1 bg-background border border-outline/60 font-terminal text-xs"
            data-testid="release-date"
          >
        </label>
        <div class="flex gap-2 justify-end">
          <button
            type="button"
            class="px-3 py-2 border border-outline/60 font-terminal text-[9px] uppercase tracking-terminal text-tertiary"
            @click="formOpen = false"
          >
            Cancel
          </button>
          <button
            type="button"
            class="px-3 py-2 border border-primary text-primary font-terminal text-[9px] uppercase tracking-terminal disabled:opacity-50"
            :disabled="saving || !title.trim() || !releaseDate"
            data-testid="release-save"
            @click="create"
          >
            {{ saving ? 'Saving…' : 'Create' }}
          </button>
        </div>
      </div>
    </section>
  </div>
</template>
