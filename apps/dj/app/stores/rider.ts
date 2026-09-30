import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type {
  RiderAttachment,
  RiderAttachmentCreateInput,
  RiderAttachmentUpdateInput,
  RiderPdfExport,
  RiderTemplate,
  RiderTemplateCreateInput,
  RiderTemplateUpdateInput,
} from '../types/rider'

// useRiderStore — Pinia setup-function style. Mirrors useEpkStore
// conventions: saveStatus is a plain ref (not computed), so the
// useRiderAutosave composable can mutate it directly without the
// storeToRefs friction documented in
// `[Phase 03-epk-press-kit-builder]: Direct store property access via
// ref/watch (not storeToRefs) in EPK components`.
export const useRiderStore = defineStore('rider', () => {
  // State — templates list, attachments keyed by gigId, save indicator.
  const templates = ref<RiderTemplate[]>([])
  const attachmentsByGigId = ref<Record<string, RiderAttachment | null>>({})
  const saveStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle')

  // ── Computed ───────────────────────────────────────────────────────
  const templateById = computed(() => {
    const map = new Map<string, RiderTemplate>()
    for (const t of templates.value) map.set(t.id, t)
    return map
  })

  // ── Template actions ──────────────────────────────────────────────
  async function loadTemplates(): Promise<void> {
    const result = await $fetch<{ data: RiderTemplate[] }>('/api/v1/rider/templates')
    templates.value = result.data
  }

  async function createTemplate(input: RiderTemplateCreateInput): Promise<RiderTemplate> {
    const result = await $fetch<{ data: RiderTemplate }>('/api/v1/rider/templates', {
      method: 'POST',
      body: input,
    })
    templates.value = [...templates.value, result.data]
    return result.data
  }

  async function updateTemplate(
    id: string,
    patch: RiderTemplateUpdateInput,
  ): Promise<RiderTemplate> {
    const result = await $fetch<{ data: RiderTemplate }>(`/api/v1/rider/templates/${id}`, {
      method: 'PUT',
      body: patch,
    })
    templates.value = templates.value.map(t => (t.id === id ? result.data : t))
    return result.data
  }

  async function deleteTemplate(id: string): Promise<void> {
    await $fetch(`/api/v1/rider/templates/${id}`, { method: 'DELETE' })
    templates.value = templates.value.filter(t => t.id !== id)
  }

  // ── Attachment actions ─────────────────────────────────────────────
  // loadAttachmentByGig populates the per-gig cache with null for the
  // "no attachment yet" case. Subsequent calls return the cached value
  // without re-fetching — the caller can force a refresh by calling
  // loadAttachmentByGig(gigId, true).
  async function loadAttachmentByGig(
    gigId: string,
    force = false,
  ): Promise<RiderAttachment | null> {
    if (!force && gigId in attachmentsByGigId.value) {
      return attachmentsByGigId.value[gigId]
    }
    const result = await $fetch<{ data: RiderAttachment | null }>(
      `/api/v1/rider/attachments/by-gig/${gigId}`,
    )
    attachmentsByGigId.value = { ...attachmentsByGigId.value, [gigId]: result.data }
    return result.data
  }

  async function createAttachment(
    input: RiderAttachmentCreateInput,
  ): Promise<RiderAttachment> {
    const result = await $fetch<{ data: RiderAttachment }>('/api/v1/rider/attachments', {
      method: 'POST',
      body: input,
    })
    attachmentsByGigId.value = {
      ...attachmentsByGigId.value,
      [input.gigId]: result.data,
    }
    return result.data
  }

  async function updateAttachment(
    id: string,
    patch: RiderAttachmentUpdateInput,
  ): Promise<RiderAttachment> {
    const result = await $fetch<{ data: RiderAttachment }>(`/api/v1/rider/attachments/${id}`, {
      method: 'PUT',
      body: patch,
    })
    attachmentsByGigId.value = {
      ...attachmentsByGigId.value,
      [result.data.gigId]: result.data,
    }
    return result.data
  }

  async function deleteAttachment(attachment: RiderAttachment): Promise<void> {
    await $fetch(`/api/v1/rider/attachments/${attachment.id}`, { method: 'DELETE' })
    attachmentsByGigId.value = { ...attachmentsByGigId.value, [attachment.gigId]: null }
  }

  async function exportAttachmentPdf(id: string): Promise<RiderPdfExport> {
    return await $fetch<RiderPdfExport>(`/api/v1/rider/attachments/${id}/pdf`, {
      method: 'POST',
    })
  }

  return {
    // State
    templates,
    attachmentsByGigId,
    saveStatus,
    // Computed
    templateById,
    // Template methods
    loadTemplates,
    createTemplate,
    updateTemplate,
    deleteTemplate,
    // Attachment methods
    loadAttachmentByGig,
    createAttachment,
    updateAttachment,
    deleteAttachment,
    exportAttachmentPdf,
  }
})