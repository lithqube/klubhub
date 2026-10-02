import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { RiderConflictError } from '../types/rider'
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
  const saveStatus = ref<'idle' | 'saving' | 'saved' | 'error' | 'conflict'>('idle')
  // Why the last autosave failed (the API's own words, e.g. a length limit),
  // or '' when the failure has no useful detail or nothing has failed.
  const saveError = ref('')
  // Autosave targets ("template:<id>" / "attachment:<id>") whose last save was
  // rejected with 409 and are waiting for the user to reload the latest copy.
  const conflicts = ref<string[]>([])
  // Bumped when the user discards local edits and reloads: editors hold local
  // copies of the text, and reseed from the store only on this signal (never
  // on their own saves, which would clobber text typed during a request).
  const reloadVersion = ref(0)

  // ── Computed ───────────────────────────────────────────────────────
  const templateById = computed(() => {
    const map = new Map<string, RiderTemplate>()
    for (const t of templates.value) map.set(t.id, t)
    return map
  })

  function markConflict(key: string): void {
    if (!conflicts.value.includes(key)) conflicts.value = [...conflicts.value, key]
  }
  function clearConflict(key: string): void {
    conflicts.value = conflicts.value.filter(k => k !== key)
  }

  function statusOf(e: unknown): number | undefined {
    const err = e as { statusCode?: number; status?: number }
    return err?.statusCode ?? err?.status
  }

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

  // Updates carry the updatedAt this client last saw (read here, at send
  // time, so back-to-back saves always use the token the previous save
  // returned). A 409 means someone else saved first and nothing was applied.
  async function updateTemplate(
    id: string,
    patch: RiderTemplateUpdateInput,
  ): Promise<RiderTemplate> {
    const cached = templates.value.find(t => t.id === id)
    if (!cached) throw new Error('Rider template is not loaded.')
    let result: { data: RiderTemplate }
    try {
      result = await $fetch<{ data: RiderTemplate }>(`/api/v1/rider/templates/${id}`, {
        method: 'PUT',
        body: { ...patch, updatedAt: cached.updatedAt },
      })
    } catch (e) {
      if (statusOf(e) === 409) throw new RiderConflictError('template', id)
      throw e
    }
    templates.value = templates.value.map(t => (t.id === id ? result.data : t))
    return result.data
  }

  // Re-read one template, replacing the cached copy (and its token). A 404
  // means it was deleted elsewhere: drop it from the list.
  async function reloadTemplate(id: string): Promise<void> {
    try {
      const result = await $fetch<{ data: RiderTemplate }>(`/api/v1/rider/templates/${id}`)
      templates.value = templates.value.map(t => (t.id === id ? result.data : t))
    } catch (e) {
      if (statusOf(e) !== 404) throw e
      templates.value = templates.value.filter(t => t.id !== id)
    }
  }

  async function deleteTemplate(id: string): Promise<void> {
    await $fetch(`/api/v1/rider/templates/${id}`, { method: 'DELETE' })
    templates.value = templates.value.filter(t => t.id !== id)
    // The server clears template_id on the attachments made from it (their
    // text and updatedAt are untouched); do the same to the cached copies so
    // their status label stops claiming a template that no longer exists.
    const next: Record<string, RiderAttachment | null> = {}
    for (const [gigId, a] of Object.entries(attachmentsByGigId.value)) {
      next[gigId] = a && a.templateId === id ? { ...a, templateId: null } : a
    }
    attachmentsByGigId.value = next
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
    const cached = Object.values(attachmentsByGigId.value).find(a => a?.id === id)
    if (!cached) throw new Error('Rider attachment is not loaded.')
    let result: { data: RiderAttachment }
    try {
      result = await $fetch<{ data: RiderAttachment }>(`/api/v1/rider/attachments/${id}`, {
        method: 'PUT',
        body: { ...patch, updatedAt: cached.updatedAt },
      })
    } catch (e) {
      if (statusOf(e) === 409) throw new RiderConflictError('attachment', id)
      throw e
    }
    attachmentsByGigId.value = {
      ...attachmentsByGigId.value,
      [result.data.gigId]: result.data,
    }
    return result.data
  }

  // Re-read one attachment, replacing the cached copy (and its token). A 404
  // means it was detached elsewhere: clear it from the per-gig cache.
  async function reloadAttachment(id: string): Promise<void> {
    try {
      const result = await $fetch<{ data: RiderAttachment }>(`/api/v1/rider/attachments/${id}`)
      attachmentsByGigId.value = { ...attachmentsByGigId.value, [result.data.gigId]: result.data }
    } catch (e) {
      if (statusOf(e) !== 404) throw e
      const entry = Object.entries(attachmentsByGigId.value).find(([, a]) => a?.id === id)
      if (entry) attachmentsByGigId.value = { ...attachmentsByGigId.value, [entry[0]]: null }
    }
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
    saveError,
    conflicts,
    reloadVersion,
    // Computed
    templateById,
    // Template methods
    loadTemplates,
    createTemplate,
    updateTemplate,
    deleteTemplate,
    reloadTemplate,
    // Attachment methods
    loadAttachmentByGig,
    createAttachment,
    updateAttachment,
    deleteAttachment,
    reloadAttachment,
    exportAttachmentPdf,
    // Autosave conflict tracking (409 on a stale updatedAt)
    markConflict,
    clearConflict,
  }
})