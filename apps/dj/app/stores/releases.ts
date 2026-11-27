import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  ChecklistItem,
  Deadline,
  PlatformLink,
  Release,
  ReleaseInput,
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
  }
})
