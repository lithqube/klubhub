<script setup lang="ts">
// /releases/[id] — one release: its workflow state, deadlines, platform links
// and the promo checklist.

import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useReleaseStore } from '~/stores/releases'
import { RELEASE_STATUS_LABELS, type ReleaseStatus } from '~/types/release'
import ReleaseStatusStepper from '~/components/release/ReleaseStatusStepper.vue'
import ReleaseDeadlineList from '~/components/release/ReleaseDeadlineList.vue'
import ReleaseLinkList from '~/components/release/ReleaseLinkList.vue'
import ReleaseChecklist from '~/components/release/ReleaseChecklist.vue'

const route = useRoute()
const store = useReleaseStore()
const { current, deadlines, links, checklist, loading, error } = storeToRefs(store)

const busy = ref(false)
const id = computed(() => String(route.params.id ?? ''))

async function load(): Promise<void> {
  busy.value = true
  await store.fetchRelease(id.value)
  await Promise.all([
    store.fetchDeadlines(id.value),
    store.fetchLinks(id.value),
    store.fetchChecklist(id.value),
  ])
  busy.value = false
}

onMounted(load)

async function move(status: ReleaseStatus): Promise<void> {
  busy.value = true
  await store.transitionRelease(id.value, status)
  busy.value = false
}

async function addDeadline(name: string, dueDate: string): Promise<void> {
  await store.addDeadline(id.value, name, dueDate)
}

async function completeDeadline(deadlineId: string): Promise<void> {
  await store.completeDeadline(id.value, deadlineId)
}

async function removeDeadline(deadlineId: string): Promise<void> {
  await store.removeDeadline(id.value, deadlineId)
}

async function saveLink(platform: string, url: string): Promise<void> {
  await store.setLink(id.value, platform as never, url)
}

async function removeLink(platform: string): Promise<void> {
  await store.removeLink(id.value, platform as never)
}

async function addStep(label: string): Promise<void> {
  await store.addChecklistItem(id.value, label)
}

async function toggleStep(itemId: string, done: boolean): Promise<void> {
  await store.toggleChecklistItem(id.value, itemId, done)
}

async function removeStep(itemId: string): Promise<void> {
  await store.removeChecklistItem(id.value, itemId)
}

async function saveAsDefault(): Promise<void> {
  await store.saveChecklistAsTemplate(id.value)
}
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <header class="px-4 pt-6 pb-4 border-b border-outline/40">
      <h1 class="font-terminal text-lg tracking-terminal uppercase">
        {{ current?.title ?? 'Release' }}
      </h1>
      <p v-if="current" class="font-terminal text-[9px] uppercase tracking-terminal text-tertiary">
        {{ current.artists || 'no artists yet' }} · {{ RELEASE_STATUS_LABELS[current.status] }} ·
        {{ current.release_date }}
      </p>
    </header>

    <p v-if="error" class="px-4 py-2 text-[10px] uppercase tracking-terminal text-error">
      {{ error }}
    </p>

    <section v-if="loading" class="px-4 py-6 font-terminal text-[10px] uppercase tracking-terminal text-tertiary">
      Loading release…
    </section>

    <section v-else-if="current" class="px-4 py-4 space-y-4">
      <ReleaseStatusStepper :status="current.status" :busy="busy" @change="move" />

      <ReleaseDeadlineList
        :deadlines="deadlines"
        :busy="busy"
        @add="addDeadline"
        @complete="completeDeadline"
        @remove="removeDeadline"
      />

      <ReleaseLinkList :links="links" :busy="busy" @save="saveLink" @remove="removeLink" />

      <ReleaseChecklist
        :items="checklist"
        :busy="busy"
        @add="addStep"
        @toggle="toggleStep"
        @remove="removeStep"
        @save-as-default="saveAsDefault"
      />
    </section>
  </div>
</template>
