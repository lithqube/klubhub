import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Organization } from '~/types/org'
import { apiFetch } from '~/utils/api'

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

  return { org, loading, error, fetchOrg }
})
