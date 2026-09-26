<script setup lang="ts">
import { useVenueStore } from '~/stores/venue'
import type { ApiError, VenueInput } from '~/types/event'

useHead({ title: 'New venue' })

const store = useVenueStore()
const busy = ref(false)
const serverError = ref<ApiError | null>(null)

async function create(input: VenueInput) {
  busy.value = true
  serverError.value = null
  try {
    const v = await store.saveVenue(input)
    await navigateTo(`/venues/${v.id}`)
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
        <h1 class="page-title">NEW VENUE</h1>
        <div class="page-sub">REUSED BY EVERY NIGHT YOU PLAN HERE</div>
      </div>
    </div>
    <div class="page-body">
      <p v-if="serverError && !serverError.field" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);margin-bottom:12px;">
        {{ serverError.error === 'no_role_grant' ? 'Your role can\'t add venues. Ask an owner, admin or booker.' : 'Couldn\'t save the venue. Try again.' }}
      </p>
      <VenueForm :busy="busy" :server-error="serverError" submit-label="SAVE VENUE" @submit="create" @cancel="navigateTo('/venues')" />
    </div>
  </div>
</template>
