<script setup lang="ts">
// RiderAttachmentEditor — per-gig copy editor. Loaded from /rider?gig=<id>
// deep link. Identical shape to RiderTemplateEditor but for an
// attachment; the route ?gig= param is consumed in pages/rider.vue.

import { onBeforeUnmount, ref, watch } from 'vue'
import { useRiderAutosave } from '~/composables/useRiderAutosave'
import { useRiderStore } from '~/stores/rider'
import type { RiderAttachment } from '~/types/rider'
import RiderConflictNotice from './RiderConflictNotice.vue'
import RiderSectionField from './RiderSectionField.vue'

const props = defineProps<{ attachment: RiderAttachment }>()

const store = useRiderStore()
const { scheduleSave, flush, cancel } = useRiderAutosave()
// Set while a detach is under way so the unmount flush does not PUT to the
// record we are deleting (it would 404 and leave a sticky "save failed").
const detaching = ref(false)

const technical = ref(props.attachment.technical)
const hospitality = ref(props.attachment.hospitality)
const backline = ref(props.attachment.backline)
const otherNotes = ref(props.attachment.otherNotes)

// Reseed on a different attachment, or when the user explicitly discards their
// edits to take the server's copy after a conflict (not on our own saves).
// Separate sources, not one getter returning an array (a fresh array always
// counts as "changed" and would fire after every save).
watch([() => props.attachment.id, () => store.reloadVersion], () => {
  technical.value = props.attachment.technical
  hospitality.value = props.attachment.hospitality
  backline.value = props.attachment.backline
  otherNotes.value = props.attachment.otherNotes
})

function onSection(field: 'technical' | 'hospitality' | 'backline' | 'otherNotes', value: string): void {
  const snapshot = {
    technical: technical.value,
    hospitality: hospitality.value,
    backline: backline.value,
    otherNotes: otherNotes.value,
    [field]: value,
  }
  scheduleSave(
    { kind: 'attachment', id: props.attachment.id },
    {
      technical: snapshot.technical,
      hospitality: snapshot.hospitality,
      backline: snapshot.backline,
      otherNotes: snapshot.otherNotes,
    },
  )
}

const exporting = ref(false)
async function onExport(): Promise<void> {
  exporting.value = true
  try {
    const res = await store.exportAttachmentPdf(props.attachment.id)
    if (res.downloadUrl) window.open(res.downloadUrl, '_blank', 'noopener')
  } finally {
    exporting.value = false
  }
}

async function onDetach(): Promise<void> {
  if (!confirm('Detach rider? Per-gig overrides will be lost.')) return
  detaching.value = true
  const target = { kind: 'attachment' as const, id: props.attachment.id }
  try {
    // Stop pending saves and let an in-flight one land before the DELETE.
    await cancel(target)
    await store.deleteAttachment(props.attachment)
    await navigateTo('/rider')
  } catch (error) {
    // The attachment still exists: the editor text is still on screen, so
    // queue it again rather than silently dropping the unsaved edits.
    detaching.value = false
    scheduleSave(target, {
      technical: technical.value,
      hospitality: hospitality.value,
      backline: backline.value,
      otherNotes: otherNotes.value,
    })
    throw error
  }
}

onBeforeUnmount(() => {
  if (detaching.value) return
  void flush({ kind: 'attachment', id: props.attachment.id })
})
</script>

<template>
  <div class="space-y-4">
    <RiderConflictNotice :id="attachment.id" kind="attachment" />
    <div class="glass-panel p-4 flex items-center justify-between">
      <div>
        <p class="text-xs tracking-terminal text-tertiary uppercase font-terminal">RIDER ATTACHMENT</p>
        <p class="font-terminal text-on-surface text-sm tracking-terminal mt-1">
          {{ attachment.templateId ? 'FROM TEMPLATE' : 'CUSTOM' }}
        </p>
      </div>
      <div class="flex gap-2">
        <button
          type="button"
          class="btn-hud btn-hud-cta"
          :disabled="exporting"
          data-testid="rider-attachment-export"
          @click="onExport"
        >{{ exporting ? 'EXPORTING…' : 'EXPORT PDF' }}</button>
        <button
          type="button"
          class="btn-hud btn-hud-ghost"
          data-testid="rider-attachment-detach"
          @click="onDetach"
        >DETACH</button>
      </div>
    </div>

    <RiderSectionField
      section="technical"
      :model-value="technical"
      @update:model-value="(v: string) => { technical = v; onSection('technical', v) }"
    />
    <RiderSectionField
      section="hospitality"
      :model-value="hospitality"
      @update:model-value="(v: string) => { hospitality = v; onSection('hospitality', v) }"
    />
    <RiderSectionField
      section="backline"
      :model-value="backline"
      @update:model-value="(v: string) => { backline = v; onSection('backline', v) }"
    />
    <RiderSectionField
      section="otherNotes"
      :model-value="otherNotes"
      @update:model-value="(v: string) => { otherNotes = v; onSection('otherNotes', v) }"
    />
  </div>
</template>