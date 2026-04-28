import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Contact } from '../types/gig'

export const useContactStore = defineStore('contact', () => {
  const contacts = ref<Contact[]>([])

  async function fetchAutocomplete(
    q: string,
    limit = 5
  ): Promise<Contact[]> {
    try {
      const data = await $fetch<Contact[]>(
        `/api/v1/contacts/autocomplete?q=${encodeURIComponent(q)}&limit=${limit}`
      )
      contacts.value = Array.isArray(data) ? data : (data as any).data || []
      return contacts.value
    } catch (e) {
      console.error('fetchAutocomplete failed:', e)
      return []
    }
  }

  return { contacts, fetchAutocomplete }
})
