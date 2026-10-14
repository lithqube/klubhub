import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { BillingProfile, UpdateBillingProfileRequest } from '../types/finance'
import { toFinanceError, type FinanceApiError } from './invoice'

const URL = '/api/v1/finance/billing-profile'

function unwrap<T>(res: unknown): T {
  if (res && typeof res === 'object' && 'data' in (res as object)) return (res as { data: T }).data
  return res as T
}

/** The DJ's billing identity (supplier on invoices): GET / PUT under an `updated_at` token. */
export const useBillingProfileStore = defineStore('billingProfile', () => {
  const profile = ref<BillingProfile | null>(null)
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<FinanceApiError | null>(null)

  async function fetchProfile(): Promise<BillingProfile | null> {
    loading.value = true
    error.value = null
    try {
      profile.value = unwrap<BillingProfile>(await $fetch<unknown>(URL))
      loaded.value = true
      return profile.value
    } catch (e) {
      error.value = toFinanceError(e)
      return null
    } finally {
      loading.value = false
    }
  }

  /** Throws FinanceApiError (400 field errors, 409 conflict); state changes only from the response. */
  async function updateProfile(input: UpdateBillingProfileRequest): Promise<BillingProfile> {
    try {
      const saved = unwrap<BillingProfile>(await $fetch<unknown>(URL, { method: 'PUT', body: { ...input } }))
      profile.value = saved
      return saved
    } catch (e) {
      throw toFinanceError(e)
    }
  }

  return { profile, loading, loaded, error, fetchProfile, updateProfile }
})
