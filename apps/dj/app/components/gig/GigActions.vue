<script setup lang="ts">
import { useGigStore } from '../../stores/gig'
import type { Gig } from '../../types/gig'

const props = defineProps<{
  gig: Gig
}>()

const gigStore = useGigStore()
const toastMessage = ref('')
const showToast = ref(false)

async function copyICalUrl() {
  const url = gigStore.generateICalUrl()
  try {
    await navigator.clipboard.writeText(
      `${window.location.origin}${url}`
    )
    toastMessage.value = 'iCal feed URL copied!'
    showToast.value = true
    setTimeout(() => (showToast.value = false), 2000)
  } catch (e) {
    toastMessage.value = 'Failed to copy URL'
    showToast.value = true
    setTimeout(() => (showToast.value = false), 2000)
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
      class="btn-hud"
      style="padding:4px 8px;font-size:9px;letter-spacing:.04em;"
      title="Copy iCal feed URL"
      @click="copyICalUrl"
    >
      ICAL
    </button>
    <button
      v-if="gig.status === 'confirmed'"
      class="btn-hud"
      style="padding:4px 8px;font-size:9px;letter-spacing:.04em;"
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
        style="position:fixed;bottom:80px;left:50%;transform:translateX(-50%);z-index:9999;padding:8px 16px;background:rgba(0,0,0,0.9);border:1px solid var(--color-primary);border-radius:2px;"
      >
        <span class="section-lbl" style="font-size:10px;color:var(--color-primary);">
          {{ toastMessage }}
        </span>
      </div>
    </Transition>
  </div>
</template>
