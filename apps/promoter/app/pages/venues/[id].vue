<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useSessionStore } from '~/stores/session'
import { useVenueStore } from '~/stores/venue'
import type { ApiError, Venue, VenueInput, VenueProtected } from '~/types/event'

const route = useRoute()
const store = useVenueStore()
const { canEditEvents } = storeToRefs(useSessionStore())
const id = computed(() => String(route.params.id))

const { data: venue, error: loadError, refresh } = await useAsyncData(() => `venue-${id.value}`, () => store.getVenue(id.value), { watch: [id] })
useHead(() => ({ title: venue.value?.name ?? 'Venue' }))

const busy = ref(false)
const serverError = ref<ApiError | null>(null)
const notice = ref<string | null>(null)
const formKey = ref(0)

// Decrypted values live only in this page's memory, never in a store or on disk.
const revealed = ref<VenueProtected | null>(null)
const revealing = ref(false)
const revealError = ref<string | null>(null)

async function reveal() {
  revealing.value = true
  revealError.value = null
  try {
    revealed.value = await store.revealVenue(id.value)
  } catch (e) {
    const err = e as ApiError
    revealError.value = ({
      mfa_required: 'Showing protected fields needs two-factor sign-in.',
      no_role_grant: 'Your role can\'t see protected venue fields.',
    } as Record<string, string>)[err.error] ?? 'Couldn\'t show the protected fields. Try again.'
  } finally {
    revealing.value = false
  }
}

async function save(input: VenueInput) {
  busy.value = true
  serverError.value = null
  notice.value = null
  try {
    venue.value = await store.saveVenue(input, id.value) as Venue
    revealed.value = null
    notice.value = 'SAVED'
    formKey.value++
  } catch (e) {
    serverError.value = e as ApiError
  } finally {
    busy.value = false
  }
}

const archiveError = ref<string | null>(null)
async function archive() {
  if (!venue.value || !window.confirm(`Archive ${venue.value.name}? Past events keep it; it leaves the venue list.`)) return
  archiveError.value = null
  try {
    await store.archiveVenue(id.value)
    await navigateTo('/venues')
  } catch (e) {
    const err = e as ApiError
    archiveError.value = err.error === 'venue_in_use'
      ? 'Upcoming events still use this venue. Move or cancel them first.'
      : 'Couldn\'t archive the venue.'
  }
}
</script>

<template>
  <div>
    <div class="page-header" style="flex-wrap:wrap;gap:10px;">
      <div style="min-width:0;">
        <NuxtLink to="/venues" class="data-frag" style="font-size:9px;text-decoration:none;">← VENUES</NuxtLink>
        <h1 class="page-title" style="margin:4px 0 0;overflow-wrap:anywhere;">{{ venue?.name ?? 'VENUE' }}</h1>
        <div v-if="venue" class="page-sub">{{ [venue.city, venue.country].filter(Boolean).join(', ') }} · {{ venue.timezone }}</div>
      </div>
      <button v-if="venue && canEditEvents" type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;color:var(--color-error);" @click="archive">ARCHIVE</button>
    </div>
    <div class="page-body space-y-3">
      <p v-if="loadError" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);">
        {{ loadError.statusCode === 404 ? 'This venue doesn\'t exist or was archived.' : 'Couldn\'t load the venue.' }}
        <button v-if="loadError.statusCode !== 404" type="button" class="btn-hud btn-hud-ghost" @click="refresh()">RETRY</button>
      </p>
      <p v-if="archiveError" role="alert" class="glass" style="padding:12px 16px;border-left:3px solid var(--color-error);">{{ archiveError }}</p>
      <p v-if="notice" role="status" class="data-frag" style="font-size:9px;">{{ notice }}</p>
      <p v-if="!canEditEvents && venue" class="glass" style="padding:10px 14px;font-size:13px;">
        READ ONLY · Owners, admins and bookers can change venues.
      </p>
      <VenueForm
        v-if="venue" :key="formKey" :initial="venue" :busy="busy" :server-error="serverError"
        :revealed="revealed" :revealing="revealing" :reveal-error="revealError" :read-only="!canEditEvents"
        submit-label="SAVE CHANGES" @submit="save" @cancel="formKey++; revealed = null" @reveal="reveal"
      />
    </div>
  </div>
</template>
