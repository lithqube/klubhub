<script setup lang="ts">
// RiderAttachmentEditor — per-gig copy editor. Loaded from /rider?gig=<id>
// deep link. Identical shape to RiderTemplateEditor but for an
// attachment; the route ?gig= param is consumed in pages/rider.vue.

import { ref, watch } from 'vue'
import { useRiderAutosave } from '~/composables/useRiderAutosave'
import { useRiderStore } from '~/stores/rider'
import type { RiderAttachment } from '~/types/rider'
import RiderSectionField from './RiderSectionField.vue'

const props = defineProps<{ attachment: RiderAttachment }>()

const store = useRiderStore()
const { scheduleSave, flush } = useRiderAutosave()

const technical = ref(props.attachment.technical)
const hospitality = ref(props.attachment.hospitality)
const backline = ref(props.attachment.backline)
const otherNotes = ref(props.attachment.otherNotes)

watch(() => props.attachment.id, () => {
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
  await store.deleteAttachment(props.attachment)
  await navigateTo('/rider')
}

onBeforeUnmount(() => {
  void flush({ kind: 'attachment', id: props.attachment.id })
})
</script>

<template>
  <div class="space-y-4">
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