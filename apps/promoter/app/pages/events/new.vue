<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import { useVenueStore } from '~/stores/venue'
import type { ApiError, EventInput } from '~/types/event'

useHead({ title: 'New event' })

const events = useEventStore()
const venueStore = useVenueStore()
const { venues } = storeToRefs(venueStore)
await useAsyncData('venues', () => venueStore.fetchVenues())

const busy = ref(false)
const serverError = ref<ApiError | null>(null)

async function create(input: EventInput) {
  busy.value = true
  serverError.value = null
  try {
    const d = await events.createEvent(input)
    await navigateTo(`/events/${d.id}/lineup`)
  } catch (e) {
    serverError.value = e as ApiError
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">NEW EVENT</h1>
        <div class="page-sub">SAVED AS A DRAFT · NOTHING IS PUBLIC YET</div>
      </div>
    </div>
    <div class="page-body">
      <EventForm :venues="venues" :busy="busy" :server-error="serverError" submit-label="CREATE & ADD LINEUP →" @submit="create" @cancel="navigateTo('/events')" />
    </div>
  </div>
</template>
