<script setup lang="ts">
// RiderAttachmentCard — compact summary used in both the gig form and
// the attachments tab. Mirrors the COMPACT UX in 04.5-05-PLAN.md
// §RiderAttachmentCard.

import { computed, onMounted, ref, watch } from 'vue'
import { useRiderStore } from '~/stores/rider'
import type { Gig } from '~/types/gig'
import { riderStatusLabel } from '~/types/rider'
import { openPdf } from '~/utils/openPdf'
import { riderErrorMessage } from '~/utils/riderErrors'
import AttachRiderDialog from './AttachRiderDialog.vue'

const props = defineProps<{ gig: Gig }>()
const emit = defineEmits<{
  'open-editor': [gigId: string]
  exported: []
}>()

const store = useRiderStore()
const attachment = computed(() => store.attachmentsByGigId[props.gig.id] ?? null)
const template = computed(() =>
  attachment.value?.templateId ? store.templateById.get(attachment.value.templateId) ?? null : null,
)
const status = computed(() => riderStatusLabel(attachment.value, template.value))
const exporting = ref(false)
const exportError = ref('')
const showAttach = ref(false)

// The card is used where nothing else has loaded the rider (the gig form),
// so it hydrates its own gig; cached gigs are not refetched. Without this it
// shows "NO RIDER ATTACHED" for a gig that has one.
async function hydrate(): Promise<void> {
  try {
    const att = await store.loadAttachmentByGig(props.gig.id)
    if (att?.templateId && store.templates.length === 0) await store.loadTemplates()
  } catch {
    // Leave the card as-is; the user can still open the editor.
  }
}
onMounted(hydrate)
watch(() => props.gig.id, hydrate)

async function onExport(): Promise<void> {
  if (!attachment.value) return
  exporting.value = true
  exportError.value = ''
  try {
    const res = await store.exportAttachmentPdf(attachment.value.id)
    if (res.downloadUrl) {
      openPdf(res.downloadUrl)
      emit('exported')
    }
  } catch (e) {
    exportError.value = riderErrorMessage(e, 'Could not export the PDF.')
  } finally {
    exporting.value = false
  }
}

async function onDetach(): Promise<void> {
  if (!attachment.value) return
  if (!confirm('Detach rider? Per-gig overrides will be lost.')) return
  await store.deleteAttachment(attachment.value)
}

async function onAttached(): Promise<void> {
  showAttach.value = false
  await store.loadAttachmentByGig(props.gig.id, true)
}

const toneColor = computed(() => {
  switch (status.value.tone) {
    case 'accent':
      return 'var(--green)'
    case 'amber':
      return 'var(--color-primary)'
    default:
      return 'var(--color-tertiary)'
  }
})
</script>

<template>
  <div data-testid="rider-attachment-card" class="glass-panel p-4 space-y-3">
    <div class="flex items-center justify-between">
      <div>
        <p class="font-terminal text-tertiary text-xs uppercase tracking-terminal">
          {{ new Date(gig.date).toLocaleDateString() }} · {{ gig.venue?.toUpperCase() ?? 'VENUE TBA' }}
        </p>
        <p
          class="font-terminal uppercase text-xs tracking-terminal mt-1"
          :style="{ color: toneColor }"
          :data-testid="`rider-status-${gig.id}`"
        >
          {{ status.label }}
        </p>
      </div>
    </div>

    <div v-if="attachment" class="space-y-1 font-terminal text-xs text-on-surface-variant">
      <div v-if="attachment.technical">
        <span class="text-tertiary">TECH:</span> {{ attachment.technical.split('\n')[0] }}
      </div>
      <div v-if="attachment.hospitality">
        <span class="text-tertiary">HOSPITALITY:</span> {{ attachment.hospitality.split('\n')[0] }}
      </div>
    </div>

    <div class="flex flex-wrap gap-2">
      <button
        v-if="!attachment"
        type="button"
        class="btn-hud btn-hud-cta"
        :data-testid="`rider-attach-btn-${gig.id}`"
        @click="showAttach = true"
      >+ ATTACH RIDER</button>

      <template v-else>
        <button
          type="button"
          class="btn-hud btn-hud-ghost"
          :data-testid="`rider-open-editor-${gig.id}`"
          @click="emit('open-editor', gig.id)"
        >OPEN EDITOR</button>
        <button
          type="button"
          class="btn-hud btn-hud-cta"
          :disabled="exporting"
          :data-testid="`rider-export-pdf-${gig.id}`"
          @click="onExport"
        >{{ exporting ? 'EXPORTING…' : 'EXPORT PDF' }}</button>
        <button
          type="button"
          class="btn-hud btn-hud-ghost"
          :data-testid="`rider-detach-${gig.id}`"
          @click="onDetach"
        >DETACH</button>
      </template>
    </div>

    <p v-if="exportError" role="alert" class="text-xs text-error" data-testid="rider-export-error">{{ exportError }}</p>

    <AttachRiderDialog
      v-if="showAttach"
      :open="showAttach"
      :gig-id="gig.id"
      @close="showAttach = false"
      @attached="onAttached"
    />
  </div>
</template>