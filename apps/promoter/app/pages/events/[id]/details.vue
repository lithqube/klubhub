<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import { useVenueStore } from '~/stores/venue'
import type { ApiError, EventInput } from '~/types/event'

const store = useEventStore()
const venueStore = useVenueStore()
const { current } = storeToRefs(store)
const { venues } = storeToRefs(venueStore)
await useAsyncData('venues', () => venueStore.fetchVenues())

const busy = ref(false)
const serverError = ref<ApiError | null>(null)
const notice = ref<string | null>(null)
const formKey = ref(0)

const cutNames = computed(() => {
  const ids = new Set(serverError.value?.entries ?? [])
  return current.value?.lineup.filter(l => ids.has(l.id)).map(l => l.display_name) ?? []
})

async function save(input: EventInput) {
  if (!current.value) return
  busy.value = true
  serverError.value = null
  notice.value = null
  try {
    await store.updateEvent(current.value.id, current.value.version, input)
    notice.value = 'SAVED'
    formKey.value++
  } catch (e) {
    const err = e as ApiError
    if (err.error === 'invalid') serverError.value = err
    else serverError.value = { ...err, field: undefined }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="current" class="space-y-3">
    <p v-if="notice" role="status" class="data-frag" style="font-size:9px;">{{ notice }}</p>
    <div v-if="serverError?.error === 'cuts_sets'" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);">
      <div class="section-lbl">NOT SAVED · THE NEW TIMES CUT {{ cutNames.length }} {{ cutNames.length === 1 ? 'SET' : 'SETS' }}</div>
      <p style="margin:6px 0 0;font-size:13px;">{{ cutNames.join(', ') }}. Move them in the timetable first.</p>
      <NuxtLink :to="`/events/${current.id}/lineup`" class="btn-hud btn-hud-ghost" style="margin-top:8px;min-height:44px;">ADJUST TIMETABLE FIRST</NuxtLink>
    </div>
    <div v-else-if="serverError?.error === 'version_conflict'" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);">
      SOMEONE ELSE SAVED THIS EVENT.
      <button type="button" class="btn-hud btn-hud-ghost" @click="store.fetchEvent(current.id).then(() => { serverError = null; formKey++ })">RELOAD</button>
    </div>
    <EventForm
      :key="formKey" :venues="venues" :initial="current" :busy="busy"
      :server-error="serverError?.error === 'invalid' ? serverError : null"
      submit-label="SAVE CHANGES" @submit="save" @cancel="formKey++"
    />
  </div>
</template>
