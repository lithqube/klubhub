<script setup lang="ts">
// /rider — two-tab page (TEMPLATES / ATTACHMENTS). Supports a
// ?gig=<id> deep link from the gig form's "OPEN EDITOR" button — opens
// the per-gig attachment editor inline.

import { computed, onMounted, ref, watch } from 'vue'
import { useRiderAutosave } from '~/composables/useRiderAutosave'
import { useRiderStore } from '~/stores/rider'
import type { RiderTemplate } from '~/types/rider'
import RiderAttachmentEditor from '../components/rider/RiderAttachmentEditor.vue'
import RiderAttachmentList from '../components/rider/RiderAttachmentList.vue'
import RiderPageHeader from '../components/rider/RiderPageHeader.vue'
import RiderTemplateDialog from '../components/rider/RiderTemplateDialog.vue'
import RiderTemplateEditor from '../components/rider/RiderTemplateEditor.vue'
import RiderTemplateList from '../components/rider/RiderTemplateList.vue'

useHead({ title: 'Rider — KlubHub DJ' })

const store = useRiderStore()
const { flushAll } = useRiderAutosave()

const tab = ref<'templates' | 'attachments'>('templates')
const selectedTemplateId = ref<string | null>(null)
const showCreateDialog = ref(false)
const route = useRoute()

onMounted(async () => {
  try {
    await store.loadTemplates()
  } catch {
    // Mock handlers may not be loaded yet — fall through, the
    // /templates GET will surface the error in the UI.
  }
})

// Deep link: /rider?gig=<id> opens the per-gig attachment editor.
const attachmentGigId = computed(() => {
  const q = route.query.gig
  return typeof q === 'string' ? q : ''
})

const selectedAttachment = computed(() => {
  if (!attachmentGigId.value) return null
  return store.attachmentsByGigId[attachmentGigId.value] ?? null
})

watch(attachmentGigId, async (id) => {
  if (id) {
    await store.loadAttachmentByGig(id, true)
  }
}, { immediate: true })

const selectedTemplate = computed<RiderTemplate | null>(() => {
  if (!selectedTemplateId.value) return null
  return store.templateById.get(selectedTemplateId.value) ?? null
})

function selectFirstTemplate(): void {
  if (selectedTemplateId.value) return
  if (store.templates.length > 0) {
    selectedTemplateId.value = store.templates[0]!.id
  }
}

watch(() => store.templates.length, () => {
  selectFirstTemplate()
}, { immediate: true })

onBeforeUnmount(() => {
  void flushAll()
})
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">
    <div class="page-header">
      <RiderPageHeader />
    </div>

    <!-- ?gig=<id> deep-link mode: attachment editor only, no tabs -->
    <div v-if="attachmentGigId" class="page-body" style="overflow-y:auto;padding:16px;">
      <RiderAttachmentEditor v-if="selectedAttachment" :attachment="selectedAttachment" />
      <p v-else class="text-tertiary font-terminal uppercase text-xs">
        No rider attached for this gig.
      </p>
    </div>

    <template v-else>
      <div class="tabs-bar" data-testid="rider-tabs">
        <button
          type="button"
          class="tab-item"
          :class="{ 'is-active': tab === 'templates' }"
          data-testid="rider-tab-templates"
          @click="tab = 'templates'"
        >TEMPLATES</button>
        <button
          type="button"
          class="tab-item"
          :class="{ 'is-active': tab === 'attachments' }"
          data-testid="rider-tab-attachments"
          @click="tab = 'attachments'"
        >ATTACHMENTS</button>
      </div>

      <div class="page-body" style="display:flex;gap:16px;overflow:hidden;padding:16px;">
        <!-- TEMPLATES tab: list + editor split -->
        <template v-if="tab === 'templates'">
          <div style="width:260px;flex-shrink:0;display:flex;flex-direction:column;gap:12px;">
            <button
              type="button"
              class="btn-hud btn-hud-cta"
              style="width:100%;"
              data-testid="rider-new-template"
              @click="showCreateDialog = true"
            >+ NEW TEMPLATE</button>
            <div style="overflow-y:auto;">
              <RiderTemplateList
                :templates="store.templates.map(t => ({ id: t.id, name: t.name }))"
                @select="(id: string) => (selectedTemplateId = id)"
              />
            </div>
          </div>
          <div style="flex:1;overflow-y:auto;">
            <RiderTemplateEditor v-if="selectedTemplate" :template="selectedTemplate" />
            <p v-else class="text-tertiary font-terminal uppercase text-xs">
              Select a template or create a new one.
            </p>
          </div>
        </template>

        <!-- ATTACHMENTS tab -->
        <div v-else style="flex:1;overflow-y:auto;">
          <RiderAttachmentList />
        </div>
      </div>
    </template>

    <RiderTemplateDialog
      :open="showCreateDialog"
      @close="showCreateDialog = false"
    />
  </div>
</template>

<style scoped>
.tabs-bar {
  display: flex;
  gap: 0;
  padding: 0 16px;
  border-bottom: 1px solid var(--color-border);
}
.tab-item {
  padding: 10px 16px;
  font-family: var(--font-terminal);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--color-tertiary);
  background: transparent;
  border: 0;
  border-bottom: 2px solid transparent;
  cursor: pointer;
}
.tab-item.is-active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}
</style>