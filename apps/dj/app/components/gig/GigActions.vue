<script setup lang="ts">
import { useGigStore } from '../../stores/gig'
import { useInvoiceStore, toFinanceError } from '../../stores/invoice'
import { isActiveInvoice } from '../../utils/invoiceDisplay'
import type { Gig } from '../../types/gig'
import { DEMO_ICAL_HINT, useDemo } from '../../composables/useDemo'

const props = defineProps<{
  gig: Gig
}>()

const gigStore = useGigStore()
const toastMessage = ref('')
const showToast = ref(false)

const { isDemo, downloadCalendar } = useDemo()

async function copyICalUrl() {
  if (isDemo) return downloadCalendar().catch(() => flash('Calendar download failed'))
  const url = gigStore.generateICalUrl()
  try {
    await navigator.clipboard.writeText(
      `${window.location.origin}${url}`
    )
    toastMessage.value = 'iCal feed URL copied!'
    showToast.value = true
    setTimeout(() => (showToast.value = false), 2000)
  } catch {
    toastMessage.value = 'Failed to copy URL'
    showToast.value = true
    setTimeout(() => (showToast.value = false), 2000)
  }
}

// ── Invoice: open the gig's active invoice, or start one preset to this gig ──
const invoiceStore = useInvoiceStore()
const invoiceBusy = ref(false)
const canInvoice = computed(() => ['confirmed', 'advanced', 'played'].includes(props.gig.status))

function flash(message: string) {
  toastMessage.value = message
  showToast.value = true
  setTimeout(() => (showToast.value = false), 2000)
}

async function openInvoice() {
  invoiceBusy.value = true
  try {
    const list = await invoiceStore.fetchInvoicesForGig(props.gig.id)
    const active = list.find(isActiveInvoice)
    if (active) void invoiceStore.openDetail(active.id)
    else invoiceStore.openCreate(props.gig.id)
  } catch (e) {
    flash(toFinanceError(e).message)
  } finally {
    invoiceBusy.value = false
  }
}

async function downloadPdf() {
  const icalSecret = useRuntimeConfig().public.icalSecret || ''
  const url = `/api/v1/gigs/${props.gig.id}/pdf?secret=${icalSecret}`
  try {
    const blob = await $fetch<Blob>(url, { responseType: 'blob' })
    const blobUrl = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = blobUrl
    a.download = `booking-confirmation-${props.gig.id}.pdf`
    a.click()
    URL.revokeObjectURL(blobUrl)
  } catch (e) {
    console.error('PDF download failed:', e)
  }
}
</script>

<template>
  <div style="display:flex;gap:6px;align-items:center;">
    <button
      class="btn-hud btn-hud-xs"
      :title="isDemo ? DEMO_ICAL_HINT : 'Copy iCal feed URL'"
      @click="copyICalUrl"
    >
      ICAL
    </button>
    <button
      v-if="canInvoice"
      type="button"
      class="btn-hud btn-hud-xs gig-action-btn"
      title="Open or create the invoice for this gig"
      :aria-label="`Invoice for ${gig.event_name || gig.venue || 'this gig'}`"
      :disabled="invoiceBusy"
      @click.stop="openInvoice"
    >
      {{ invoiceBusy ? '…' : 'INVOICE' }}
    </button>
    <button
      v-if="gig.status === 'confirmed'"
      class="btn-hud btn-hud-xs"
      title="Download booking confirmation PDF"
      @click="downloadPdf"
    >
      PDF
    </button>

    <!-- Toast notification -->
    <Transition
      enter-active-class="transition-opacity duration-200"
      leave-active-class="transition-opacity duration-200"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div
        v-if="showToast"
        class="glass"
        style="position:fixed;bottom:80px;left:50%;transform:translateX(-50%);z-index:9999;padding:8px 16px;box-shadow:var(--shadow-glow-primary);"
      >
        <span class="section-lbl" style="font-size:10px;color:var(--color-primary);">
          {{ toastMessage }}
        </span>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
@media (max-width: 768px) {
  .gig-action-btn { min-height: 44px; height: 44px; }
}
</style>
