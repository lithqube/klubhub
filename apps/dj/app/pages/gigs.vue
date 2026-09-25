<script setup lang="ts">
import { Plus, Calendar, List, Download } from 'lucide-vue-next'
import { useGigStore } from '../stores/gig'
import { storeToRefs } from 'pinia'
import type { Gig } from '../types/gig'
import GigStatsCards from '../components/gig/GigStatsCards.vue'
import GigListRow from '../components/gig/GigListRow.vue'
import GigCalendarView from '../components/gig/GigCalendarView.vue'
import GigFormDialog from '../components/gig/GigFormDialog.vue'
import RaEventImport from '../components/gig/RaEventImport.vue'
import { useFeatures } from '../composables/useFeatures'
import { DEMO_ICAL_HINT, useDemo } from '../composables/useDemo'
import InvoiceWorkspace from '../components/finance/InvoiceWorkspace.vue'


useHead({ title: 'Gigs — KlubHub DJ' })

const gigStore = useGigStore()
// Licensed edition feature (docs/EDITIONS.md): hidden unless enabled.
const raImportEnabled = useFeatures().isEnabled('raImport')
const { gigs, loading, filters } = storeToRefs(gigStore)

const viewMode = ref<'list' | 'calendar'>('list')
const formOpen = ref(false)
const selectedGig = ref<Gig | null>(null)

// Fetch gigs on mount
onMounted(async () => {
  await gigStore.fetchGigs()
  openGigFromQuery()
})

// Deep link from the invoice sheet: /gigs?gig=<id> opens that gig's form.
const route = useRoute()
function openGigFromQuery() {
  const id = typeof route.query.gig === 'string' ? route.query.gig : ''
  if (!id) return
  const gig = gigs.value.find((g) => g.id === id)
  if (gig) openEditGig(gig)
  // Drop the query so following the same link again reopens the form.
  void navigateTo({ path: '/gigs' }, { replace: true })
}
watch(() => route.query.gig, () => openGigFromQuery())

function openAddGig() {
  selectedGig.value = null
  formOpen.value = true
}

function openEditGig(gig: Gig) {
  selectedGig.value = gig
  formOpen.value = true
}

function onGigSaved() {
  gigStore.fetchGigs()
  selectedGig.value = null
  formOpen.value = false
}

function onGigClickFromCalendar(gig: Gig) {
  openEditGig(gig)
}

const { isDemo, downloadCalendar } = useDemo()

async function copyICalUrl() {
  if (isDemo) return downloadCalendar().catch((e) => console.warn('Calendar download failed:', e))
  const url = gigStore.generateICalUrl()
  try {
    await navigator.clipboard.writeText(`${window.location.origin}${url}`)
    alert('iCal feed URL copied to clipboard!')
  } catch (e) {
    console.error('Failed to copy iCal URL:', e)
  }
}

// Filter handlers
function setStatusFilter(status: string) {
  gigStore.setFilter('status', status)
  gigStore.fetchGigs()
}

function setCityFilter(city: string) {
  gigStore.setFilter('city', city)
  gigStore.fetchGigs()
}

function clearFilters() {
  gigStore.clearFilters()
  gigStore.fetchGigs()
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">GIGS</div>
        <div class="page-sub">GIG TRACKER · SCHEDULE & HISTORY</div>
      </div>
      <div style="display:flex;gap:8px;align-items:center;">
        <button
          class="btn-hud btn-hud-ghost"
          style="padding:0 14px;"
          :title="isDemo ? DEMO_ICAL_HINT : 'Copy iCal feed URL'"
          @click="copyICalUrl"
        >
          <Download style="width:12px;height:12px;" aria-hidden="true" />
          EXPORT ICAL
        </button>
        <button
          class="btn-hud btn-hud-cta"
          style="padding:0 14px;"
          @click="openAddGig"
        >
          <Plus style="width:12px;height:12px;" aria-hidden="true" />
          ADD GIG
        </button>
      </div>
    </div>

    <!-- Scrollable body -->
    <div class="page-body">

      <!-- Stats row -->
      <GigStatsCards />

      <!-- RA Event Import Panel (licensed edition feature) -->
      <div v-if="raImportEnabled" style="margin-bottom:12px;">
        <RaEventImport />
      </div>

      <!-- View toggle + filters -->
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;">
        <div style="display:flex;gap:6px;">
          <button
            class="btn-hud btn-hud-ghost btn-hud-sm"
            :style="viewMode === 'list' ? 'background:color-mix(in srgb, var(--color-primary) 8%, transparent);border-color:color-mix(in srgb, var(--color-primary) 25%, transparent);color:var(--color-primary);' : ''"
            @click="viewMode = 'list'"
          >
            <List style="width:10px;height:10px;display:inline;margin-right:4px;" />
            LIST
          </button>
          <button
            class="btn-hud btn-hud-ghost btn-hud-sm"
            :style="viewMode === 'calendar' ? 'background:color-mix(in srgb, var(--color-primary) 8%, transparent);border-color:color-mix(in srgb, var(--color-primary) 25%, transparent);color:var(--color-primary);' : ''"
            @click="viewMode = 'calendar'"
          >
            <Calendar style="width:10px;height:10px;display:inline;margin-right:4px;" />
            CALENDAR
          </button>
        </div>

        <!-- Quick filters -->
        <div style="display:flex;gap:6px;align-items:center;">
          <select
            :value="filters.status"
            class="hud-input"
            style="width:auto;height:28px;"
            @change="setStatusFilter(($event.target as HTMLSelectElement).value)"
          >
            <option value="">ALL STATUS</option>
            <option value="inquiry">INQUIRY</option>
            <option value="confirmed">CONFIRMED</option>
            <option value="advanced">ADVANCED</option>
            <option value="played">PLAYED</option>
            <option value="cancelled">CANCELLED</option>
          </select>
          <input
            :value="filters.city"
            type="text"
            class="hud-input"
            placeholder="City..."
            style="width:80px;height:28px;"
            @input="setCityFilter(($event.target as HTMLInputElement).value)"
          >
          <button
            v-if="filters.status || filters.city"
            class="btn-hud btn-hud-ghost btn-hud-xs"
            @click="clearFilters"
          >
            CLEAR
          </button>
        </div>
      </div>

      <!-- Gig list / calendar -->
      <div>
        <div class="section-lbl" style="margin-bottom:8px;">GIG SCHEDULE</div>

        <!-- Loading state -->
        <div v-if="loading" class="glass" style="padding:32px 24px;text-align:center;">
          <div class="section-lbl" style="font-size:10px;color:var(--color-tertiary);">LOADING GIGS...</div>
        </div>

        <!-- Empty state -->
        <div
          v-else-if="gigs.length === 0"
          class="glass"
          style="padding:32px 24px;text-align:center;border:1px dashed color-mix(in srgb, var(--color-primary) 15%, transparent);"
        >
          <div
            style="font-family:var(--font-command);font-size:12px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;margin-bottom:6px;"
          >
            NO GIGS FOUND
          </div>
          <div
            style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);"
          >
            ADD YOUR FIRST GIG TO GET STARTED
          </div>
        </div>

        <!-- List view -->
        <template v-else-if="viewMode === 'list'">
          <div class="glass" style="overflow:hidden;">
            <GigListRow
              v-for="gig in gigs"
              :key="gig.id"
              :gig="gig"
              @edit="openEditGig"
            />
          </div>
        </template>

        <!-- Calendar view -->
        <template v-else>
          <GigCalendarView
            :gigs="gigs"
            @gig-click="onGigClickFromCalendar"
          />
        </template>
      </div>

    </div>

    <!-- Gig form dialog -->
    <GigFormDialog
      v-model:open="formOpen"
      :gig="selectedGig"
      @saved="onGigSaved"
    />

    <!-- Invoice create dialog + detail sheet (opened from the gig form strip) -->
    <InvoiceWorkspace />
  </div>
</template>
