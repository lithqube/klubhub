<script setup lang="ts">
// /rider — two-tab page (TEMPLATES / ATTACHMENTS). Supports a
// ?gig=<id> deep link from the gig form's "OPEN EDITOR" button — opens
// the per-gig attachment editor inline.
//
// Editors are mounted only once fresh data has arrived. An editor keeps a
// local copy of the text, seeded when it mounts; mounting it from a stale
// cache and then refreshing the store underneath it would leave it editing
// old text under a new updatedAt token and silently overwrite newer content.

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { hasAnyUnsavedRiderEdits, useRiderAutosave } from '~/composables/useRiderAutosave'
import { useRiderStore } from '~/stores/rider'
import type { Gig } from '~/types/gig'
import type { RiderTemplate } from '~/types/rider'
import { riderErrorMessage } from '~/utils/riderErrors'
import RiderAttachmentCard from '../components/rider/RiderAttachmentCard.vue'
import RiderAttachmentEditor from '../components/rider/RiderAttachmentEditor.vue'
import RiderAttachmentList from '../components/rider/RiderAttachmentList.vue'
import RiderPageHeader from '../components/rider/RiderPageHeader.vue'
import RiderTemplateDialog from '../components/rider/RiderTemplateDialog.vue'
import RiderTemplateEditor from '../components/rider/RiderTemplateEditor.vue'
import RiderTemplateList from '../components/rider/RiderTemplateList.vue'

useHead({ title: 'Rider — KlubHub DJ' })

type LoadState = 'loading' | 'ready' | 'error'

const store = useRiderStore()
const { flushAll } = useRiderAutosave()
const route = useRoute()

const tab = ref<'templates' | 'attachments'>('templates')
const selectedTemplateId = ref<string | null>(null)
const showCreateDialog = ref(false)

// ── templates ──
const templatesState = ref<LoadState>('loading')
const templatesError = ref('')

async function loadTemplates(): Promise<void> {
  templatesState.value = 'loading'
  templatesError.value = ''
  try {
    await store.loadTemplates()
    templatesState.value = 'ready'
  } catch (e) {
    templatesError.value = riderErrorMessage(e, 'Could not load your rider templates.')
    templatesState.value = 'error'
  }
}

// ── deep link: /rider?gig=<id> ──
const attachmentGigId = computed(() => {
  const q = route.query.gig
  return typeof q === 'string' ? q : ''
})
const attachmentState = ref<LoadState>('loading')
const attachmentError = ref('')

async function loadAttachment(id: string): Promise<void> {
  attachmentState.value = 'loading'
  attachmentError.value = ''
  try {
    await store.loadAttachmentByGig(id, true)
    if (id !== attachmentGigId.value) return // the link changed while loading
    attachmentState.value = 'ready'
  } catch (e) {
    if (id !== attachmentGigId.value) return
    attachmentError.value = riderErrorMessage(e, 'Could not load this rider.')
    attachmentState.value = 'error'
  }
}

watch(attachmentGigId, (id) => {
  if (id) void loadAttachment(id)
}, { immediate: true })

const selectedAttachment = computed(() => {
  if (!attachmentGigId.value) return null
  return store.attachmentsByGigId[attachmentGigId.value] ?? null
})
// Only `id` is known here. The card loads and attaches the rider by itself and
// leaves out the date/venue line when the gig has no `date`.
const deepLinkGig = computed(() => ({ id: attachmentGigId.value }) as Gig)

// ── selection ──
const selectedTemplate = computed<RiderTemplate | null>(() => {
  if (!selectedTemplateId.value) return null
  return store.templateById.get(selectedTemplateId.value) ?? null
})

// Keep the selection valid as templates come and go (loaded, created,
// deleted here or elsewhere): drop a template that no longer exists, and pick
// the first one when nothing is selected.
watch(() => store.templates.map(t => t.id).join('|'), () => {
  const ids = store.templates.map(t => t.id)
  if (selectedTemplateId.value && !ids.includes(selectedTemplateId.value)) selectedTemplateId.value = null
  if (!selectedTemplateId.value && ids.length > 0) selectedTemplateId.value = ids[0]!
}, { immediate: true })

function onTemplateCreated(t: RiderTemplate): void {
  tab.value = 'templates'
  selectedTemplateId.value = t.id
}

// ── leaving the page ──
// Autosave is debounced, so edits made in the last moments are still queued.
// Warn before a reload/close would drop them, and push them out when the tab
// is hidden (a tab switch, or the first step of closing it on mobile).
function onBeforeUnload(e: BeforeUnloadEvent): void {
  if (!hasAnyUnsavedRiderEdits()) return
  e.preventDefault()
  e.returnValue = ''
}
function onVisibilityChange(): void {
  if (document.visibilityState === 'hidden') void flushAll()
}

onMounted(() => {
  void loadTemplates()
  window.addEventListener('beforeunload', onBeforeUnload)
  document.addEventListener('visibilitychange', onVisibilityChange)
})
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  void flushAll()
})
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">
    <div class="page-header">
      <RiderPageHeader />
    </div>

    <!-- ?gig=<id> deep-link mode: that gig's rider only, no tabs -->
    <div v-if="attachmentGigId" class="page-body" style="overflow-y:auto;padding:16px;" data-testid="rider-deep-link">
      <NuxtLink to="/rider" class="rider-back" data-testid="rider-back">← ALL RIDERS</NuxtLink>

      <p v-if="attachmentState === 'loading'" class="rider-note" role="status" data-testid="rider-deep-link-loading">LOADING RIDER…</p>

      <div v-else-if="attachmentState === 'error'" class="space-y-2" data-testid="rider-deep-link-error">
        <p class="rider-note text-error" role="alert">{{ attachmentError }}</p>
        <button type="button" class="btn-hud" data-testid="rider-deep-link-retry" @click="loadAttachment(attachmentGigId)">RETRY</button>
      </div>

      <RiderAttachmentEditor v-else-if="selectedAttachment" :attachment="selectedAttachment" />

      <div v-else class="space-y-3" data-testid="rider-deep-link-empty">
        <p class="rider-note">No rider is attached to this gig yet.</p>
        <RiderAttachmentCard :gig="deepLinkGig" @open-editor="() => {}" />
      </div>
    </div>

    <template v-else>
      <div class="tabs-bar" role="tablist" data-testid="rider-tabs">
        <button
          type="button"
          role="tab"
          :aria-selected="tab === 'templates'"
          class="tab-item"
          :class="{ 'is-active': tab === 'templates' }"
          data-testid="rider-tab-templates"
          @click="tab = 'templates'"
        >TEMPLATES</button>
        <button
          type="button"
          role="tab"
          :aria-selected="tab === 'attachments'"
          class="tab-item"
          :class="{ 'is-active': tab === 'attachments' }"
          data-testid="rider-tab-attachments"
          @click="tab = 'attachments'"
        >ATTACHMENTS</button>
      </div>

      <!-- Stacked on a phone, side by side from md up -->
      <div class="page-body rider-split">
        <!-- TEMPLATES tab: list + editor split -->
        <template v-if="tab === 'templates'">
          <div class="rider-sidebar">
            <button
              type="button"
              class="btn-hud btn-hud-cta"
              style="width:100%;"
              data-testid="rider-new-template"
              @click="showCreateDialog = true"
            >+ NEW TEMPLATE</button>
            <div class="rider-sidebar-list">
              <p v-if="templatesState === 'loading'" class="rider-note" role="status" data-testid="rider-templates-loading">LOADING TEMPLATES…</p>
              <div v-else-if="templatesState === 'error'" class="space-y-2" data-testid="rider-templates-error">
                <p class="rider-note text-error" role="alert">{{ templatesError }}</p>
                <button type="button" class="btn-hud" data-testid="rider-templates-retry" @click="loadTemplates">RETRY</button>
              </div>
              <RiderTemplateList
                v-else
                :templates="store.templates.map(t => ({ id: t.id, name: t.name }))"
                :selected-id="selectedTemplateId"
                @select="(id: string) => (selectedTemplateId = id)"
              />
            </div>
          </div>
          <div class="rider-main">
            <RiderTemplateEditor v-if="templatesState === 'ready' && selectedTemplate" :key="selectedTemplate.id" :template="selectedTemplate" />
            <p v-else-if="templatesState === 'ready'" class="rider-note">
              {{ store.templates.length === 0 ? 'Create a template to reuse the same rider on many gigs.' : 'Select a template or create a new one.' }}
            </p>
          </div>
        </template>

        <!-- ATTACHMENTS tab -->
        <div v-else class="rider-main">
          <RiderAttachmentList />
        </div>
      </div>
    </template>

    <RiderTemplateDialog
      :open="showCreateDialog"
      @close="showCreateDialog = false"
      @created="onTemplateCreated"
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
.rider-note {
  font-family: var(--font-terminal);
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--color-tertiary);
}
.rider-back {
  display: inline-block;
  margin-bottom: 12px;
  font-family: var(--font-terminal);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--color-primary);
}

/* Phone: one column that scrolls as a whole. md and up: list beside editor. */
.rider-split {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  overflow-y: auto;
}
.rider-sidebar {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
}
.rider-sidebar-list {
  max-height: 40vh;
  overflow-y: auto;
}
.rider-main {
  flex: 1;
  min-width: 0;
}
@media (min-width: 768px) {
  .rider-split {
    flex-direction: row;
    overflow: hidden;
  }
  .rider-sidebar {
    width: 260px;
    flex-shrink: 0;
  }
  .rider-sidebar-list {
    max-height: none;
  }
  .rider-main {
    overflow-y: auto;
  }
}
</style>
