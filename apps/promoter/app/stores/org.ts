import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ApiError } from '~/types/event'
import type { Organization, OrgProfile } from '~/types/org'
import { apiFetch, toApiError } from '~/utils/api'

export const useOrgStore = defineStore('org', () => {
  const org = ref<Organization | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchOrg(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      org.value = await apiFetch<Organization>('/api/v1/org')
    } catch (e) {
      org.value = null
      error.value = e instanceof Error ? e.message : 'Could not load the organisation.'
    } finally {
      loading.value = false
    }
  }

  /** Returns the updated organisation, or throws an ApiError (field + problem on 422). */
  async function updateProfile(profile: OrgProfile): Promise<Organization> {
    try {
      org.value = await apiFetch<Organization>('/api/v1/org/profile', { method: 'PUT', body: profile })
      return org.value
    } catch (e) {
      throw toApiError(e) as ApiError
    }
  }

  return { org, loading, error, fetchOrg, updateProfile }
})
