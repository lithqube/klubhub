import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  ChecklistItem,
  Deadline,
  PlatformLink,
  Release,
  ReleaseInput,
  ReleasePlatform,
  ReleaseStatus,
} from '../types/release'

// The API answers with a { data } envelope on every endpoint.
function unwrap<T>(result: T | { data: T } | null | undefined): T | null {
  if (result == null) return null
  if (typeof result === 'object' && 'data' in (result as object)) {
    return (result as { data: T }).data ?? null
  }
  return result as T
}

function unwrapList<T>(result: T[] | { data: T[] | null } | null | undefined): T[] {
  const inner = unwrap(result)
  return Array.isArray(inner) ? inner : []
}

export interface ReleaseDeadlineTimelineEntry {
  releaseId: string
  releaseTitle: string
  deadline: Deadline
}

export const useReleaseStore = defineStore('releases', () => {
  const releases = ref<Release[]>([])
  const current = ref<Release | null>(null)
  const deadlines = ref<Deadline[]>([])
  const links = ref<PlatformLink[]>([])
  const checklist = ref<ChecklistItem[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchReleases(): Promise<Release[]> {
    loading.value = true
    error.value = null
    try {
      const data = await $fetch('/api/v1/releases')
      releases.value = unwrapList<Release>(data)
      return releases.value
    } catch (e) {
      error.value = 'Failed to load releases'
      console.error('fetchReleases failed:', e)
      return []
    } finally {
      loading.value = false
    }
  }

  async function fetchRelease(id: string): Promise<Release | null> {
    loading.value = true
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}`)
      current.value = unwrap<Release>(data)
      return current.value
    } catch (e) {
      error.value = 'Failed to load the release'
      console.error('fetchRelease failed:', e)
      return null
    } finally {
      loading.value = false
    }
  }

  async function createRelease(input: ReleaseInput): Promise<Release | null> {
    error.value = null
    try {
      const data = await $fetch('/api/v1/releases', { method: 'POST', body: input })
      const created = unwrap<Release>(data)
      if (created) await fetchReleases()
      return created
    } catch (e) {
      error.value = 'Failed to create the release'
      console.error('createRelease failed:', e)
      return null
    }
  }

  async function updateRelease(id: string, input: ReleaseInput): Promise<Release | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}`, { method: 'PUT', body: input })
      const updated = unwrap<Release>(data)
      if (updated) {
        current.value = updated
        await fetchReleases()
      }
      return updated
    } catch (e) {
      error.value = 'Failed to save the release'
      console.error('updateRelease failed:', e)
      return null
    }
  }

  async function deleteRelease(id: string): Promise<boolean> {
    error.value = null
    try {
      await $fetch(`/api/v1/releases/${id}`, { method: 'DELETE' })
      current.value = null
      await fetchReleases()
      return true
    } catch (e) {
      error.value = 'Failed to delete the release'
      console.error('deleteRelease failed:', e)
      return false
    }
  }

  async function transitionRelease(id: string, status: ReleaseStatus): Promise<Release | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}/transition`, {
        method: 'POST',
        body: { status },
      })
      const updated = unwrap<Release>(data)
      if (updated) {
        current.value = updated
        await fetchReleases()
      }
      return updated
    } catch (e) {
      error.value = 'That status change is not allowed'
      console.error('transitionRelease failed:', e)
      return null
    }
  }

  async function fetchDeadlineTimeline(): Promise<ReleaseDeadlineTimelineEntry[]> {
    let failed = false
    const entries = await Promise.all(releases.value.map(async (release) => {
      try {
        const result = await $fetch(`/api/v1/releases/${release.id}/deadlines`)
        return unwrapList<Deadline>(result).map((deadline) => ({
          releaseId: release.id,
          releaseTitle: release.title,
          deadline,
        }))
      } catch (e) {
        failed = true
        console.error(`fetchDeadlineTimeline failed for ${release.id}:`, e)
        return []
      }
    }))

    if (failed) error.value = 'Some release deadlines could not be loaded'
    return entries.flat()
  }

  async function fetchDeadlines(id: string): Promise<Deadline[]> {
    try {
      const data = await $fetch(`/api/v1/releases/${id}/deadlines`)
      deadlines.value = unwrapList<Deadline>(data)
      return deadlines.value
    } catch (e) {
      console.error('fetchDeadlines failed:', e)
      deadlines.value = []
      return []
    }
  }

  async function addDeadline(id: string, name: string, dueDate: string): Promise<Deadline | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}/deadlines`, {
        method: 'POST',
        body: { name, due_date: dueDate },
      })
      const created = unwrap<Deadline>(data)
      if (created) await fetchDeadlines(id)
      return created
    } catch (e) {
      error.value = 'Failed to add the deadline'
      console.error('addDeadline failed:', e)
      return null
    }
  }

  async function completeDeadline(id: string, deadlineId: string): Promise<Deadline | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}/deadlines/${deadlineId}/complete`, {
        method: 'POST',
      })
      const updated = unwrap<Deadline>(data)
      if (updated) await fetchDeadlines(id)
      return updated
    } catch (e) {
      error.value = 'Failed to complete the deadline'
      console.error('completeDeadline failed:', e)
      return null
    }
  }

  async function removeDeadline(id: string, deadlineId: string): Promise<boolean> {
    error.value = null
    try {
      await $fetch(`/api/v1/releases/${id}/deadlines/${deadlineId}`, { method: 'DELETE' })
      await fetchDeadlines(id)
      return true
    } catch (e) {
      error.value = 'Failed to remove the deadline'
      console.error('removeDeadline failed:', e)
      return false
    }
  }

  async function fetchLinks(id: string): Promise<PlatformLink[]> {
    try {
      const data = await $fetch(`/api/v1/releases/${id}/links`)
      links.value = unwrapList<PlatformLink>(data)
      return links.value
    } catch (e) {
      console.error('fetchLinks failed:', e)
      links.value = []
      return []
    }
  }

  async function setLink(id: string, platform: ReleasePlatform, url: string): Promise<PlatformLink | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}/links`, {
        method: 'POST',
        body: { platform, url },
      })
      const saved = unwrap<PlatformLink>(data)
      if (saved) await fetchLinks(id)
      return saved
    } catch (e) {
      error.value = 'Failed to save the link'
      console.error('setLink failed:', e)
      return null
    }
  }

  async function removeLink(id: string, platform: ReleasePlatform): Promise<boolean> {
    error.value = null
    try {
      await $fetch(`/api/v1/releases/${id}/links/${platform}`, { method: 'DELETE' })
      await fetchLinks(id)
      return true
    } catch (e) {
      error.value = 'Failed to remove the link'
      console.error('removeLink failed:', e)
      return false
    }
  }

  async function fetchChecklist(id: string): Promise<ChecklistItem[]> {
    try {
      const data = await $fetch(`/api/v1/releases/${id}/checklist`)
      checklist.value = unwrapList<ChecklistItem>(data)
      return checklist.value
    } catch (e) {
      console.error('fetchChecklist failed:', e)
      checklist.value = []
      return []
    }
  }

  async function addChecklistItem(id: string, label: string): Promise<ChecklistItem | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}/checklist`, {
        method: 'POST',
        body: { label },
      })
      const created = unwrap<ChecklistItem>(data)
      if (created) await fetchChecklist(id)
      return created
    } catch (e) {
      error.value = 'Failed to add the step'
      console.error('addChecklistItem failed:', e)
      return null
    }
  }

  async function toggleChecklistItem(id: string, itemId: string, done: boolean): Promise<ChecklistItem | null> {
    error.value = null
    try {
      const data = await $fetch(`/api/v1/releases/${id}/checklist/${itemId}/toggle`, {
        method: 'POST',
        body: { done },
      })
      const updated = unwrap<ChecklistItem>(data)
      if (updated) await fetchChecklist(id)
      return updated
    } catch (e) {
      error.value = 'Failed to update the step'
      console.error('toggleChecklistItem failed:', e)
      return null
    }
  }

  async function removeChecklistItem(id: string, itemId: string): Promise<boolean> {
    error.value = null
    try {
      await $fetch(`/api/v1/releases/${id}/checklist/${itemId}`, { method: 'DELETE' })
      await fetchChecklist(id)
      return true
    } catch (e) {
      error.value = 'Failed to remove the step'
      console.error('removeChecklistItem failed:', e)
      return false
    }
  }

  async function reorderChecklist(id: string, order: string[]): Promise<boolean> {
    error.value = null
    try {
      await $fetch(`/api/v1/releases/${id}/checklist/reorder`, {
        method: 'POST',
        body: { order },
      })
      await fetchChecklist(id)
      return true
    } catch (e) {
      error.value = 'Failed to reorder the steps'
      console.error('reorderChecklist failed:', e)
      return false
    }
  }

  async function saveChecklistAsTemplate(id: string): Promise<boolean> {
    error.value = null
    try {
      await $fetch(`/api/v1/releases/${id}/checklist/save-as-default`, { method: 'POST' })
      return true
    } catch (e) {
      error.value = 'Failed to save the template'
      console.error('saveChecklistAsTemplate failed:', e)
      return false
    }
  }

  return {
    releases,
    current,
    deadlines,
    links,
    checklist,
    loading,
    error,
    fetchReleases,
    fetchRelease,
    createRelease,
    updateRelease,
    deleteRelease,
    transitionRelease,
    fetchDeadlineTimeline,
    fetchDeadlines,
    addDeadline,
    completeDeadline,
    removeDeadline,
    fetchLinks,
    setLink,
    removeLink,
    fetchChecklist,
    addChecklistItem,
    toggleChecklistItem,
    removeChecklistItem,
    reorderChecklist,
    saveChecklistAsTemplate,
  }
})
